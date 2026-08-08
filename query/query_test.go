package query_test

import (
	"testing"
	"time"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func passAll() query.Predicate[model.PullRequest] {
	return func(model.PullRequest) bool { return true }
}

func TestApply_Counts(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		{Title: "draft PR", State: model.PRStateDraft, UpdatedAt: now},
		{Title: "open PR A", State: model.PRStateOpen, UpdatedAt: now.Add(-24 * time.Hour)},
		{Title: "open PR B", State: model.PRStateOpen, UpdatedAt: now.Add(-48 * time.Hour)},
	}

	predicate, err := query.PRFilterSpec{Draft: boolPointer(false)}.Compile(nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result := query.Apply(pullRequests, predicate, query.SortSpec{}, query.GroupSpec{}, "")

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.Filtered != 2 {
		t.Errorf("Filtered = %d, want 2 (draft excluded)", result.Filtered)
	}
	if result.Shown != 2 {
		t.Errorf("Shown = %d, want 2 (no fuzzy)", result.Shown)
	}
}

func TestApply_SortApplied(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		{Title: "old", UpdatedAt: now.Add(-72 * time.Hour)},
		{Title: "new", UpdatedAt: now.Add(-24 * time.Hour)},
	}

	result := query.Apply(pullRequests, passAll(),
		query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc},
		query.GroupSpec{},
		"",
	)

	items := result.Groups[0].Items
	if items[0].Title != "new" {
		t.Errorf("expected newest first after sort, got %q", items[0].Title)
	}
}

func TestApply_FuzzyNarrowsAndReranks(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		// Sort by updated desc would rank handle-auth first (more recent).
		// Fuzzy on "auth" should re-rank: "auth service fix" scores higher (prefix).
		{Title: "handle auth somewhere in code", UpdatedAt: now.Add(-1 * time.Hour)},
		{Title: "auth service fix", UpdatedAt: now.Add(-48 * time.Hour)},
		{Title: "update readme", UpdatedAt: now},
	}

	result := query.Apply(pullRequests, passAll(),
		query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc},
		query.GroupSpec{},
		"auth",
	)

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.Filtered != 3 {
		t.Errorf("Filtered = %d, want 3 (no structural filter)", result.Filtered)
	}
	if result.Shown != 2 {
		t.Errorf("Shown = %d, want 2 (fuzzy excludes 'update readme')", result.Shown)
	}

	items := result.Groups[0].Items
	if len(items) != 2 {
		t.Fatalf("expected 2 items in group, got %d", len(items))
	}
	if items[0].Title != "auth service fix" {
		t.Errorf("expected fuzzy to rank 'auth service fix' first (prefix), got %q", items[0].Title)
	}
}

func TestApply_GroupApplied(t *testing.T) {
	pullRequests := []model.PullRequest{
		{Title: "PR1", Repo: model.Repository{Owner: "acme", Name: "api"}},
		{Title: "PR2", Repo: model.Repository{Owner: "acme", Name: "frontend"}},
		{Title: "PR3", Repo: model.Repository{Owner: "acme", Name: "api"}},
	}

	result := query.Apply(pullRequests, passAll(), query.SortSpec{}, query.GroupSpec{By: query.GroupRepo}, "")

	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
	if result.Groups[0].Key != "acme/api" {
		t.Errorf("expected acme/api first (alphabetical), got %q", result.Groups[0].Key)
	}
}

func TestApply_NilFilter(t *testing.T) {
	pullRequests := []model.PullRequest{{Title: "A"}, {Title: "B"}}

	result := query.Apply(pullRequests, nil, query.SortSpec{}, query.GroupSpec{}, "")

	if result.Total != 2 || result.Filtered != 2 {
		t.Errorf("nil filter should pass all; Total=%d Filtered=%d", result.Total, result.Filtered)
	}
}

func TestApply_EmptyInput(t *testing.T) {
	result := query.Apply(nil, passAll(), query.SortSpec{}, query.GroupSpec{}, "")

	if result.Total != 0 || result.Filtered != 0 || result.Shown != 0 {
		t.Errorf("expected all-zero counts for nil input, got %+v", result)
	}
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 group (GroupNone on empty input), got %d", len(result.Groups))
	}
}
