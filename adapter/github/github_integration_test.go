//go:build integration

// Integration tests for the GitHub adapter. Run with:
//
//	GITHUB_TOKEN=<pat> go test -tags integration -v ./adapter/github/...
//
// These tests are skipped in CI unless GITHUB_TOKEN is set and the -tags
// integration flag is passed. They hit the real GitHub API.
package github_test

import (
	"context"
	"os"
	"testing"

	"github.com/lstellway/prsm/adapter"
	githubadapter "github.com/lstellway/prsm/adapter/github"
	"github.com/lstellway/prsm/model"
)

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("skipping integration test: %s not set", key)
	}
	return value
}

func newTestAdapter(t *testing.T) *githubadapter.GitHubAdapter {
	t.Helper()
	token := requireEnv(t, "GITHUB_TOKEN")

	adapterConfig := githubadapter.Config{
		Name:  "github-integration-test",
		Token: token,
		// Use a known public repo for the integration test.
		Repos: []adapter.RepoRef{
			{Owner: "golang", Repo: "go"},
		},
	}

	githubAdapter, err := githubadapter.New(adapterConfig)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return githubAdapter
}

func TestResolveIdentity(t *testing.T) {
	githubAdapter := newTestAdapter(t)

	author, err := githubAdapter.ResolveIdentity(context.Background())
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}

	if author.Username == "" {
		t.Error("expected non-empty Username")
	}
	if author.DisplayName == "" {
		t.Error("expected non-empty DisplayName")
	}
	t.Logf("resolved identity: username=%q displayName=%q", author.Username, author.DisplayName)
}

func TestListPullRequests(t *testing.T) {
	githubAdapter := newTestAdapter(t)

	pullRequests, err := githubAdapter.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	if len(pullRequests) == 0 {
		t.Skip("no open PRs in golang/go (unlikely but possible)")
	}

	t.Logf("fetched %d pull requests", len(pullRequests))

	pullRequest := pullRequests[0]
	if pullRequest.ProviderID == "" {
		t.Error("PR.ProviderID is empty")
	}
	if pullRequest.Number == 0 {
		t.Error("PR.Number is zero")
	}
	if pullRequest.Title == "" {
		t.Error("PR.Title is empty")
	}
	if pullRequest.URL == "" {
		t.Error("PR.URL is empty")
	}
	if pullRequest.HeadSHA == "" {
		t.Error("PR.HeadSHA is empty")
	}
	if pullRequest.Provider.Kind != model.ProviderGitHub {
		t.Errorf("PR.Provider.Kind = %q, want %q", pullRequest.Provider.Kind, model.ProviderGitHub)
	}
	if pullRequest.Repo.Owner != "golang" || pullRequest.Repo.Name != "go" {
		t.Errorf("PR.Repo = %v, want {golang go}", pullRequest.Repo)
	}

	// CI should start as Pending.
	if !pullRequest.CI.IsPending() {
		t.Errorf("PR.CI should be Pending after list (loaded=%v absent=%v error=%v err=%v)",
			pullRequest.CI.IsLoaded(), pullRequest.CI.IsAbsent(), pullRequest.CI.IsError(), pullRequest.CI.Err())
	}
	// ReviewerStates should start as Pending.
	if !pullRequest.Reviews.ReviewerStates.IsPending() {
		t.Errorf("PR.Reviews.ReviewerStates should be Pending after list")
	}
}

func TestLoadCI(t *testing.T) {
	githubAdapter := newTestAdapter(t)

	pullRequests, err := githubAdapter.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(pullRequests) == 0 {
		t.Skip("no open PRs to test LoadCI against")
	}

	ciStatus, err := githubAdapter.LoadCI(context.Background(), pullRequests[0])
	if err != nil {
		t.Fatalf("LoadCI: %v", err)
	}

	t.Logf("CI status: state=%q summary=%q", ciStatus.State, ciStatus.Summary)

	if ciStatus.State != model.CIStateNone &&
		ciStatus.State != model.CIStatePassing &&
		ciStatus.State != model.CIStatePending &&
		ciStatus.State != model.CIStateFailing {
		t.Errorf("unexpected CI state: %q", ciStatus.State)
	}
}

func TestLoadReviewerStates(t *testing.T) {
	githubAdapter := newTestAdapter(t)

	pullRequests, err := githubAdapter.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(pullRequests) == 0 {
		t.Skip("no open PRs to test LoadReviewerStates against")
	}

	states, err := githubAdapter.LoadReviewerStates(context.Background(), pullRequests[0])
	if err != nil {
		t.Fatalf("LoadReviewerStates: %v", err)
	}

	t.Logf("reviewer states count: %d", len(states))

	for index, state := range states {
		if state.Reviewer.Username == "" {
			t.Errorf("states[%d].Reviewer.Username is empty", index)
		}
	}
}

func TestListPullRequestsResultConsistency(t *testing.T) {
	githubAdapter := newTestAdapter(t)

	ctx := context.Background()

	// First call — populates the httpcache store. This test only checks that two
	// back-to-back calls against the live API agree; the conditional-request
	// mechanics themselves (If-None-Match sent, 304 answered, list still served
	// from cache) are asserted by TestListPullRequestsETagRevalidation in
	// etag_test.go against an httptest server.
	firstPullRequests, err := githubAdapter.ListPullRequests(ctx)
	if err != nil {
		t.Fatalf("first ListPullRequests: %v", err)
	}
	if len(firstPullRequests) == 0 {
		t.Skip("no open PRs to verify caching against")
	}

	// Collect IDs from first call for comparison.
	firstProviderIDs := make(map[string]bool, len(firstPullRequests))
	for _, pullRequest := range firstPullRequests {
		firstProviderIDs[pullRequest.ProviderID] = true
	}

	// Second call — httpcache sends If-None-Match; GitHub returns 304 if unchanged,
	// httpcache serves the cached body. Result must be identical to the first call.
	secondPullRequests, err := githubAdapter.ListPullRequests(ctx)
	if err != nil {
		t.Fatalf("second ListPullRequests: %v", err)
	}

	if len(firstPullRequests) != len(secondPullRequests) {
		// PR list changed between calls — caching behavior is correct but inconclusive.
		t.Logf("PR count changed between calls (%d → %d); skipping ID comparison",
			len(firstPullRequests), len(secondPullRequests))
		return
	}

	for _, pullRequest := range secondPullRequests {
		if !firstProviderIDs[pullRequest.ProviderID] {
			t.Errorf("PR %s appeared in second call but not first — unexpected list change",
				pullRequest.ProviderID)
		}
	}
	t.Logf("result consistency: both calls returned %d PRs with matching IDs", len(firstPullRequests))
}
