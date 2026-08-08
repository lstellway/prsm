package query_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func prWithRepo(owner, name string) model.PullRequest {
	return model.PullRequest{Repo: model.Repository{Owner: owner, Name: name}}
}

func prWithAuthor(username string) model.PullRequest {
	return model.PullRequest{Author: model.Author{Username: username}}
}

func prWithProvider(account string) model.PullRequest {
	return model.PullRequest{Provider: model.ProviderInstance{Account: account}}
}

func prWithReviewState(state model.AggregateReviewState) model.PullRequest {
	return model.PullRequest{Reviews: model.ReviewSummary{AggregateState: state}}
}

func TestGroupBy_None(t *testing.T) {
	pullRequests := []model.PullRequest{prWithRepo("a", "x"), prWithRepo("b", "y")}

	groups := query.GroupBy(pullRequests, query.GroupSpec{By: query.GroupNone})

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Key != "" {
		t.Errorf("expected empty key for GroupNone, got %q", groups[0].Key)
	}
	if len(groups[0].Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(groups[0].Items))
	}
}

func TestGroupBy_Repo(t *testing.T) {
	pullRequests := []model.PullRequest{
		prWithRepo("acme", "api"),
		prWithRepo("acme", "frontend"),
		prWithRepo("acme", "api"),
	}

	groups := query.GroupBy(pullRequests, query.GroupSpec{By: query.GroupRepo})

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// Alphabetical: acme/api < acme/frontend
	if groups[0].Key != "acme/api" {
		t.Errorf("expected first group to be acme/api, got %q", groups[0].Key)
	}
	if len(groups[0].Items) != 2 {
		t.Errorf("expected 2 items in acme/api, got %d", len(groups[0].Items))
	}
	if groups[1].Key != "acme/frontend" {
		t.Errorf("expected second group to be acme/frontend, got %q", groups[1].Key)
	}
}

func TestGroupBy_Provider(t *testing.T) {
	pullRequests := []model.PullRequest{
		prWithProvider("github-work"),
		prWithProvider("gitlab-personal"),
		prWithProvider("github-work"),
	}

	groups := query.GroupBy(pullRequests, query.GroupSpec{By: query.GroupProvider})

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// Alphabetical
	if groups[0].Key != "github-work" {
		t.Errorf("expected github-work first, got %q", groups[0].Key)
	}
}

func TestGroupBy_Author_SortedByCount(t *testing.T) {
	pullRequests := []model.PullRequest{
		prWithAuthor("alice"),
		prWithAuthor("bob"),
		prWithAuthor("alice"),
		prWithAuthor("bob"),
		prWithAuthor("bob"),
		prWithAuthor("carol"),
	}

	groups := query.GroupBy(pullRequests, query.GroupSpec{By: query.GroupAuthor})

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// bob has 3, alice has 2, carol has 1
	if groups[0].Key != "bob" {
		t.Errorf("expected bob first (most PRs), got %q", groups[0].Key)
	}
	if groups[1].Key != "alice" {
		t.Errorf("expected alice second, got %q", groups[1].Key)
	}
	if groups[2].Key != "carol" {
		t.Errorf("expected carol last, got %q", groups[2].Key)
	}
}

func TestGroupBy_ReviewStatus_TriagePriority(t *testing.T) {
	pullRequests := []model.PullRequest{
		prWithReviewState(model.AggregateReviewStateApproved),
		prWithReviewState(model.AggregateReviewStateRequired),
		prWithReviewState(model.AggregateReviewStateCommented),
		prWithReviewState(model.AggregateReviewStateChangesRequested),
		prWithReviewState(model.AggregateReviewStateNone),
		prWithReviewState(model.AggregateReviewStateUnknown), // never computed; its own bucket
	}

	groups := query.GroupBy(pullRequests, query.GroupSpec{By: query.GroupReviewStatus})

	if len(groups) != 6 {
		t.Fatalf("expected 6 groups, got %d", len(groups))
	}

	wantOrder := []string{
		string(model.AggregateReviewStateRequired),
		string(model.AggregateReviewStateChangesRequested),
		string(model.AggregateReviewStateCommented),
		string(model.AggregateReviewStateApproved),
		"none",
		"unknown",
	}
	for index, wantKey := range wantOrder {
		if groups[index].Key != wantKey {
			t.Errorf("position %d: got %q, want %q", index, groups[index].Key, wantKey)
		}
	}
}

func TestGroupBy_ValidateForResource(t *testing.T) {
	groupSpec := query.GroupSpec{By: query.GroupReviewStatus}

	if err := groupSpec.ValidateForResource("pr"); err != nil {
		t.Errorf("expected GroupReviewStatus to be valid for pr, got error: %v", err)
	}
	if err := groupSpec.ValidateForResource("issue"); err == nil {
		t.Error("expected GroupReviewStatus to be invalid for issue resource type")
	}
}

func TestGroupBy_ZeroSpec(t *testing.T) {
	pullRequests := []model.PullRequest{prWithRepo("a", "x")}

	groups := query.GroupBy(pullRequests, query.GroupSpec{})

	if len(groups) != 1 {
		t.Fatalf("expected 1 group for zero spec, got %d", len(groups))
	}
}
