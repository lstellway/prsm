package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// ---------------------------------------------------------------------------
// TestListPullRequestsETagRevalidation
// ---------------------------------------------------------------------------

// recordedRequest captures what the test server observed for a single request
// so the test can assert on conditional-request behavior after the calls return.
type recordedRequest struct {
	ifNoneMatch string
	status      int
}

// prKey is the comparable subset of model.PullRequest used to prove the
// cache-served result is the same list as the origin-served one.
// model.PullRequest itself is not comparable (it embeds LoadResult fields).
type prKey struct {
	providerID   string
	number       int
	title        string
	headSHA      string
	sourceBranch string
	targetBranch string
	author       string
	state        model.PRState
	repo         model.Repository
}

func prKeys(pullRequests []model.PullRequest) []prKey {
	keys := make([]prKey, len(pullRequests))
	for index, pullRequest := range pullRequests {
		keys[index] = prKey{
			providerID:   pullRequest.ProviderID,
			number:       pullRequest.Number,
			title:        pullRequest.Title,
			headSHA:      pullRequest.HeadSHA,
			sourceBranch: pullRequest.SourceBranch,
			targetBranch: pullRequest.TargetBranch,
			author:       pullRequest.Author.Username,
			state:        pullRequest.State,
			repo:         pullRequest.Repo,
		}
	}
	return keys
}

// TestListPullRequestsETagRevalidation verifies that the httpcache transport
// wired up in newHTTPClient performs real conditional requests.
//
// The adapter is constructed through New() — not a hand-built gogithub.Client —
// so the request travels the production path (oauth2 transport → httpcache
// transport → origin). A client built with gogithub.WithAuthToken bypasses
// newHTTPClient entirely and would pass this test with caching removed.
//
// The server answers the second call with 304 and an empty body, so the list
// the adapter returns on that call can only have come from the cache.
func TestListPullRequestsETagRevalidation(t *testing.T) {
	const etag = `"prsm-test-etag"`

	listBody, err := json.Marshal([]minimalGHPR{makePR(1), makePR(2), makePR(3)})
	if err != nil {
		t.Fatalf("marshal PR list: %v", err)
	}

	var mutex sync.Mutex
	var seen []recordedRequest

	server := httptest.NewServer(http.HandlerFunc(
		func(responseWriter http.ResponseWriter, request *http.Request) {
			mutex.Lock()
			status := http.StatusOK
			if len(seen) > 0 {
				status = http.StatusNotModified
			}
			seen = append(seen, recordedRequest{
				ifNoneMatch: request.Header.Get("If-None-Match"),
				status:      status,
			})
			mutex.Unlock()

			responseWriter.Header().Set("ETag", etag)
			// max-age=0 + must-revalidate forces the cache to revalidate against the
			// origin on the second call instead of serving the stored body with no
			// network request at all. GitHub's own max-age=60 would make the second
			// call a pure cache hit — a different path from the one under test.
			responseWriter.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")

			if status == http.StatusNotModified {
				responseWriter.WriteHeader(http.StatusNotModified)
				return
			}
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.Write(listBody) //nolint:errcheck
		}))
	defer server.Close()

	githubAdapter, err := New(Config{
		Name:    "test",
		Token:   "fake-token",
		BaseURL: server.URL,
		Repos:   []adapter.RepoRef{{Owner: "owner", Repo: "repo"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()

	first, err := githubAdapter.ListPullRequests(ctx)
	if err != nil {
		t.Fatalf("first ListPullRequests: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("first ListPullRequests returned %d PRs, want 3", len(first))
	}

	// The error is checked after the request assertions: when the cache is
	// missing, the unconditional 304 surfaces as a go-github error, and the
	// absent If-None-Match is the more diagnostic failure to report.
	second, secondErr := githubAdapter.ListPullRequests(ctx)

	mutex.Lock()
	requests := slices.Clone(seen)
	mutex.Unlock()

	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want exactly 2 (%+v)", len(requests), requests)
	}
	if requests[0].ifNoneMatch != "" {
		t.Errorf("first request sent If-None-Match %q, want none", requests[0].ifNoneMatch)
	}
	if requests[1].ifNoneMatch != etag {
		t.Errorf("second request sent If-None-Match %q, want %q — httpcache did not revalidate",
			requests[1].ifNoneMatch, etag)
	}
	if requests[1].status != http.StatusNotModified {
		t.Errorf("server answered second request with %d, want %d",
			requests[1].status, http.StatusNotModified)
	}

	if secondErr != nil {
		t.Fatalf("second ListPullRequests: %v", secondErr)
	}

	if len(second) != len(first) {
		t.Fatalf("second ListPullRequests returned %d PRs, want %d — 304 body was not served from cache",
			len(second), len(first))
	}
	if got, want := prKeys(second), prKeys(first); !slices.Equal(got, want) {
		t.Errorf("cached result differs from origin result:\n got: %+v\nwant: %+v", got, want)
	}

	// Guard against the degenerate "both calls returned empty" pass.
	for index, pullRequest := range second {
		if pullRequest.ProviderID == "" || pullRequest.Number == 0 || pullRequest.Title == "" || pullRequest.HeadSHA == "" {
			t.Errorf("second[%d] is not fully populated: %+v", index, prKeys(second)[index])
		}
	}
}
