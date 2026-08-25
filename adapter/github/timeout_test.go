package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lstellway/prsm/model"
)

// newStalledSecondPageServer serves firstPageBody as page 1 (advertising page
// 2 via a Link header, the pagination scheme every endpoint under test here
// uses), then blocks past delay before answering page 2 at all. delay is
// chosen well beyond the adapter's configured PaginationTimeout, so by the
// time this handler would respond, the calling request has already been
// cancelled and never reads it — the response body on that path is
// unreachable and does not need to be well-formed.
func newStalledSecondPageServer(firstPageBody []byte, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(responseWriter http.ResponseWriter, request *http.Request) {
			page := request.URL.Query().Get("page")
			responseWriter.Header().Set("Content-Type", "application/json")
			if page == "" || page == "1" {
				responseWriter.Header().Set("Link", fmt.Sprintf(`<%s?page=2>; rel="next"`, request.URL.Path))
				responseWriter.Write(firstPageBody) //nolint:errcheck
				return
			}
			time.Sleep(delay)
		}))
}

// Every test below uses the same bound: a 50ms pagination timeout against a
// page-2 delay an order of magnitude longer, and asserts the call returns in
// well under the delay — proving the deadline, not the slow page, is what
// ended the call.
const (
	testPaginationTimeout = 50 * time.Millisecond
	testPageDelay         = 500 * time.Millisecond
	testElapsedBudget     = 300 * time.Millisecond
)

func TestListRepoPullRequestsPaginationTimeout(t *testing.T) {
	page1Body, err := json.Marshal([]minimalGHPR{makePR(1), makePR(2)})
	if err != nil {
		t.Fatalf("marshal page 1: %v", err)
	}

	server := newStalledSecondPageServer(page1Body, testPageDelay)
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:              "test",
		Token:             "fake-token",
		BaseURL:           server.URL,
		Repos:             []RepoRef{{Owner: "owner", Repo: "repo"}},
		PaginationTimeout: testPaginationTimeout,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	started := time.Now()
	pullRequests, err := githubAdapter.listRepoPullRequests(context.Background(), "owner", "repo")
	elapsed := time.Since(started)

	if elapsed > testElapsedBudget {
		t.Errorf("listRepoPullRequests took %s, want well under the %s page-2 delay — "+
			"the pagination timeout did not bound the call", elapsed, testPageDelay)
	}
	if len(pullRequests) != 2 {
		t.Errorf("got %d partial pull requests, want 2 (page 1's results should survive the page-2 timeout)",
			len(pullRequests))
	}
	assertDeadlineExceeded(t, err)
}

func TestLoadCIPaginationTimeout(t *testing.T) {
	page1Body := []byte(`{"total_count":1,"check_runs":[{"id":1,"status":"completed","conclusion":"success"}]}`)

	server := newStalledSecondPageServer(page1Body, testPageDelay)
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:              "test",
		Token:             "fake-token",
		BaseURL:           server.URL,
		PaginationTimeout: testPaginationTimeout,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pullRequestRef := model.PullRequestRef{
		Repo:    model.Repository{Owner: "owner", Name: "repo"},
		Number:  1,
		HeadSHA: "deadbeef",
	}

	started := time.Now()
	ciStatus, err := githubAdapter.LoadCI(context.Background(), pullRequestRef)
	elapsed := time.Since(started)

	if elapsed > testElapsedBudget {
		t.Errorf("LoadCI took %s, want well under the %s page-2 delay", elapsed, testPageDelay)
	}
	// Page 1's single passing run is reflected in the partial status even
	// though the call as a whole failed.
	if ciStatus.State != model.CIStatePassing {
		t.Errorf("partial CIStatus.State = %q, want %q", ciStatus.State, model.CIStatePassing)
	}
	assertDeadlineExceeded(t, err)
}

func TestLoadReviewerStatesPaginationTimeout(t *testing.T) {
	page1Body := []byte(`[{"id":1,"user":{"login":"alice"},"state":"APPROVED"}]`)

	server := newStalledSecondPageServer(page1Body, testPageDelay)
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:              "test",
		Token:             "fake-token",
		BaseURL:           server.URL,
		PaginationTimeout: testPaginationTimeout,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pullRequestRef := model.PullRequestRef{
		Repo:   model.Repository{Owner: "owner", Name: "repo"},
		Number: 1,
	}

	started := time.Now()
	reviewerStates, err := githubAdapter.LoadReviewerStates(context.Background(), pullRequestRef)
	elapsed := time.Since(started)

	if elapsed > testElapsedBudget {
		t.Errorf("LoadReviewerStates took %s, want well under the %s page-2 delay", elapsed, testPageDelay)
	}
	if len(reviewerStates) != 1 {
		t.Errorf("got %d partial reviewer states, want 1 (page 1's review should survive the page-2 timeout)",
			len(reviewerStates))
	}
	assertDeadlineExceeded(t, err)
}

// assertDeadlineExceeded fails the test unless err is non-nil and its chain
// reaches context.DeadlineExceeded — the pagination timeout's underlying
// cause, still recoverable through paginationError's %w wrapping.
func assertDeadlineExceeded(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error when the pagination deadline fires, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v does not wrap context.DeadlineExceeded", err)
	}
}
