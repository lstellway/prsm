package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lstellway/prsm/adapter"
)

// TestResolveIdentityConcurrentWithInstance exercises the exact shape ADR-009's
// assembly layer produces: identity is resolved at startup while fetches that
// stamp each resource with Instance() are already fanning out across providers.
//
// Before the fix this failed under -race — ResolveIdentity wrote the account
// field while Instance() read it with no synchronization. It passes without
// -race either way, so it is only meaningful when the race detector is on.
func TestResolveIdentityConcurrentWithInstance(t *testing.T) {
	userBody, err := json.Marshal(map[string]any{
		"login":      "octocat",
		"name":       "The Octocat",
		"avatar_url": "https://example.invalid/octocat.png",
	})
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	pullRequestListBody, err := json.Marshal([]minimalGHPR{makePR(1), makePR(2)})
	if err != nil {
		t.Fatalf("marshal PR list: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(responseWriter http.ResponseWriter, request *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			// No caching: every call must reach the handler so the goroutines keep
			// interleaving for the whole run rather than settling into cache hits.
			responseWriter.Header().Set("Cache-Control", "no-store")
			// go-github rewrites an enterprise base URL to .../api/v3/, so match the
			// trailing segment rather than an absolute path.
			if strings.HasSuffix(request.URL.Path, "/user") {
				responseWriter.Write(userBody) //nolint:errcheck
				return
			}
			responseWriter.Write(pullRequestListBody) //nolint:errcheck
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

	const iterations = 50
	var waitGroup sync.WaitGroup
	waitGroup.Add(3)

	// Writer: repeated identity resolution, as a reconnect or re-auth would do.
	go func() {
		defer waitGroup.Done()
		for range iterations {
			if _, err := githubAdapter.ResolveIdentity(ctx); err != nil {
				t.Errorf("ResolveIdentity: %v", err)
				return
			}
		}
	}()

	// Reader: direct Instance() calls.
	go func() {
		defer waitGroup.Done()
		for range iterations {
			if kind := githubAdapter.Instance().Kind; kind != "github" {
				t.Errorf("Instance().Kind = %q, want %q", kind, "github")
				return
			}
		}
	}()

	// Reader: Instance() reached indirectly through the fetch path, which stamps
	// every normalized PR with it.
	go func() {
		defer waitGroup.Done()
		for range iterations {
			if _, err := githubAdapter.ListPullRequests(ctx); err != nil {
				t.Errorf("ListPullRequests: %v", err)
				return
			}
		}
	}()

	waitGroup.Wait()

	if account := githubAdapter.Instance().Account; account != "octocat" {
		t.Errorf("Instance().Account = %q, want %q", account, "octocat")
	}
}
