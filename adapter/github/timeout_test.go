package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestListPullRequestsPartialFailureAcrossRepos guards the public entry
// point every consumer actually calls: a connection configured with several
// repos must still return the PRs from repos that succeeded, even when
// another repo's fetch fails outright. listRepoPullRequests returning
// partial results is not enough on its own — ListPullRequests must not
// discard them before the caller ever sees them.
func TestListPullRequestsPartialFailureAcrossRepos(t *testing.T) {
	goodPullRequestsBody, err := json.Marshal([]minimalGHPR{makePR(1)})
	if err != nil {
		t.Fatalf("marshal good repo body: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(responseWriter http.ResponseWriter, request *http.Request) {
			if strings.Contains(request.URL.Path, "/bad-repo/") {
				http.Error(responseWriter, "internal error", http.StatusInternalServerError)
				return
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.Write(goodPullRequestsBody) //nolint:errcheck
		}))
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:    "test",
		Token:   "fake-token",
		BaseURL: server.URL,
		Repos: []RepoRef{
			{Owner: "acme", Repo: "good-repo"},
			{Owner: "acme", Repo: "bad-repo"},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pullRequests, err := githubAdapter.ListPullRequests(context.Background())

	if len(pullRequests) != 1 {
		t.Errorf("got %d pull requests, want 1 from good-repo (bad-repo's failure must not discard it)",
			len(pullRequests))
	}
	if err == nil {
		t.Fatal("expected an error reporting bad-repo's failure, got nil")
	}
	if !strings.Contains(err.Error(), "bad-repo") {
		t.Errorf("error %q does not mention the failing repo", err)
	}
}

// TestLoadCIFirstPageFailureReturnsUnknown guards against LoadCI reporting
// CIStateNone — model.CIStatus's documented claim that CI ran nowhere on
// this PR — when in fact zero check runs were fetched only because the
// first page failed. That failure has established nothing about whether CI
// exists; the correct answer is the zero-value CIStateUnknown.
func TestLoadCIFirstPageFailureReturnsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(responseWriter http.ResponseWriter, request *http.Request) {
			http.Error(responseWriter, "internal error", http.StatusInternalServerError)
		}))
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:    "test",
		Token:   "fake-token",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pullRequestRef := model.PullRequestRef{
		Repo:    model.Repository{Owner: "owner", Name: "repo"},
		Number:  1,
		HeadSHA: "deadbeef",
	}

	ciStatus, err := githubAdapter.LoadCI(context.Background(), pullRequestRef)

	if ciStatus.State != model.CIStateUnknown {
		t.Errorf("CIStatus.State = %q, want %q (CIStateNone would wrongly claim CI ran nowhere on this PR)",
			ciStatus.State, model.CIStateUnknown)
	}
	if err == nil {
		t.Fatal("expected an error from the failed first page, got nil")
	}
}

// TestPaginationErrorDistinguishesTimeoutFromCancellation guards the
// message paginationError builds: it must blame the pagination timeout only
// when ctx's own error is specifically DeadlineExceeded, not any other
// reason ctx ended (an outer caller cancelling it first, for instance).
func TestPaginationErrorDistinguishesTimeoutFromCancellation(t *testing.T) {
	t.Run("deadline_exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		<-ctx.Done()

		err := paginationError(ctx, 30*time.Second, "prefix", 3, ctx.Err())

		if !strings.Contains(err.Error(), "pagination timeout") {
			t.Errorf("error %q does not attribute the failure to the pagination timeout", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error %v does not wrap context.DeadlineExceeded", err)
		}
	})

	t.Run("outer_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := paginationError(ctx, 30*time.Second, "prefix", 3, ctx.Err())

		if strings.Contains(err.Error(), "pagination timeout") {
			t.Errorf("error %q wrongly blames the pagination timeout for an outer cancellation", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error %v does not wrap context.Canceled", err)
		}
	})
}
