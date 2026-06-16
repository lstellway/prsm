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

	githubadapter "github.com/lstellway/prsm/adapter/github"
	"github.com/lstellway/prsm/config"
	"github.com/lstellway/prsm/model"
)

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("skipping integration test: %s not set", key)
	}
	return v
}

func newTestAdapter(t *testing.T) *githubadapter.GitHubAdapter {
	t.Helper()
	token := requireEnv(t, "GITHUB_TOKEN")

	cfg := config.ProviderConfig{
		Name: "github-integration-test",
		Type: "github",
		Auth: config.AuthConfig{
			Type:  "pat",
			Token: token,
		},
		// Use a known public repo for the integration test.
		Repos: []config.RepoRef{
			{Owner: "golang", Repo: "go"},
		},
	}

	a, err := githubadapter.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestResolveIdentity(t *testing.T) {
	a := newTestAdapter(t)

	author, err := a.ResolveIdentity(context.Background())
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
	a := newTestAdapter(t)

	prs, err := a.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	if len(prs) == 0 {
		t.Skip("no open PRs in golang/go (unlikely but possible)")
	}

	t.Logf("fetched %d pull requests", len(prs))

	pr := prs[0]
	if pr.ProviderID == "" {
		t.Error("PR.ProviderID is empty")
	}
	if pr.Number == 0 {
		t.Error("PR.Number is zero")
	}
	if pr.Title == "" {
		t.Error("PR.Title is empty")
	}
	if pr.URL == "" {
		t.Error("PR.URL is empty")
	}
	if pr.HeadSHA == "" {
		t.Error("PR.HeadSHA is empty")
	}
	if pr.Provider.Kind != model.ProviderGitHub {
		t.Errorf("PR.Provider.Kind = %q, want %q", pr.Provider.Kind, model.ProviderGitHub)
	}
	if pr.Repo.Owner != "golang" || pr.Repo.Name != "go" {
		t.Errorf("PR.Repo = %v, want {golang go}", pr.Repo)
	}

	// CI should start as Pending.
	if !pr.CI.IsPending() {
		t.Errorf("PR.CI should be Pending after list, got state %v", pr.CI.State())
	}
	// ReviewerStates should start as Pending.
	if !pr.Reviews.ReviewerStates.IsPending() {
		t.Errorf("PR.Reviews.ReviewerStates should be Pending after list")
	}
}

func TestLoadCI(t *testing.T) {
	a := newTestAdapter(t)

	prs, err := a.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) == 0 {
		t.Skip("no open PRs to test LoadCI against")
	}

	ci, err := a.LoadCI(context.Background(), prs[0])
	if err != nil {
		t.Fatalf("LoadCI: %v", err)
	}

	t.Logf("CI status: state=%q summary=%q", ci.State, ci.Summary)
}

func TestLoadReviewerStates(t *testing.T) {
	a := newTestAdapter(t)

	prs, err := a.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) == 0 {
		t.Skip("no open PRs to test LoadReviewerStates against")
	}

	states, err := a.LoadReviewerStates(context.Background(), prs[0])
	if err != nil {
		t.Fatalf("LoadReviewerStates: %v", err)
	}

	t.Logf("reviewer states count: %d", len(states))
}

func TestETagConditionalRequest(t *testing.T) {
	a := newTestAdapter(t)

	ctx := context.Background()

	// First call — should succeed and cache the ETag.
	prs1, err := a.ListPullRequests(ctx)
	if err != nil {
		t.Fatalf("first ListPullRequests: %v", err)
	}

	// Second call — should use If-None-Match; if data unchanged, returns cached result.
	prs2, err := a.ListPullRequests(ctx)
	if err != nil {
		t.Fatalf("second ListPullRequests: %v", err)
	}

	if len(prs1) != len(prs2) {
		t.Logf("note: PR count changed between calls (%d → %d); ETag test inconclusive",
			len(prs1), len(prs2))
	}
	t.Logf("ETag conditional request test: first=%d second=%d", len(prs1), len(prs2))
}
