package query_test

import (
	"testing"
	"time"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func makePR(title string, updatedAt, createdAt time.Time) model.PullRequest {
	return model.PullRequest{Title: title, UpdatedAt: updatedAt, CreatedAt: createdAt}
}

func TestSort_UpdatedDesc(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("C", now.Add(-72*time.Hour), now.Add(-72*time.Hour)),
		makePR("A", now.Add(-24*time.Hour), now.Add(-24*time.Hour)),
		makePR("B", now.Add(-48*time.Hour), now.Add(-48*time.Hour)),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc})

	want := []string{"A", "B", "C"}
	for index, wantTitle := range want {
		if sorted[index].Title != wantTitle {
			t.Errorf("position %d: got %q, want %q", index, sorted[index].Title, wantTitle)
		}
	}
}

func TestSort_UpdatedAsc(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("C", now.Add(-72*time.Hour), now),
		makePR("A", now.Add(-24*time.Hour), now),
		makePR("B", now.Add(-48*time.Hour), now),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortUpdated, Direction: query.SortAsc})

	want := []string{"C", "B", "A"}
	for index, wantTitle := range want {
		if sorted[index].Title != wantTitle {
			t.Errorf("position %d: got %q, want %q", index, sorted[index].Title, wantTitle)
		}
	}
}

func TestSort_Staleness_Desc(t *testing.T) {
	// Staleness desc = most stale first (largest staleness number = oldest UpdatedAt first).
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("recent", now.Add(-24*time.Hour), now),
		makePR("stale", now.Add(-168*time.Hour), now),
		makePR("mid", now.Add(-72*time.Hour), now),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortStaleness, Direction: query.SortDesc})

	if sorted[0].Title != "stale" {
		t.Errorf("expected stale first with SortDesc, got %q", sorted[0].Title)
	}
	if sorted[2].Title != "recent" {
		t.Errorf("expected recent last with SortDesc, got %q", sorted[2].Title)
	}
}

func TestSort_Staleness_Asc(t *testing.T) {
	// Staleness asc = least stale first (smallest staleness number = most recently updated first).
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("recent", now.Add(-24*time.Hour), now),
		makePR("stale", now.Add(-168*time.Hour), now),
		makePR("mid", now.Add(-72*time.Hour), now),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortStaleness, Direction: query.SortAsc})

	if sorted[0].Title != "recent" {
		t.Errorf("expected recent first with SortAsc, got %q", sorted[0].Title)
	}
	if sorted[2].Title != "stale" {
		t.Errorf("expected stale last with SortAsc, got %q", sorted[2].Title)
	}
}

func TestSort_Staleness_DiffersFromUpdated(t *testing.T) {
	// SortStaleness desc and SortUpdated desc must produce opposite orderings.
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("old", now.Add(-72*time.Hour), now),
		makePR("new", now.Add(-24*time.Hour), now),
	}

	byUpdatedDesc := query.Sort(pullRequests, query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc})
	byStalenessDesc := query.Sort(pullRequests, query.SortSpec{By: query.SortStaleness, Direction: query.SortDesc})

	if byUpdatedDesc[0].Title != "new" {
		t.Errorf("SortUpdated desc should put newest first, got %q", byUpdatedDesc[0].Title)
	}
	if byStalenessDesc[0].Title != "old" {
		t.Errorf("SortStaleness desc should put most stale first, got %q", byStalenessDesc[0].Title)
	}
}

func TestSort_TitleAsc(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("Zebra fix", now, now),
		makePR("add feature", now, now),
		makePR("Bump version", now, now),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortTitle, Direction: query.SortAsc})

	want := []string{"add feature", "Bump version", "Zebra fix"}
	for index, wantTitle := range want {
		if sorted[index].Title != wantTitle {
			t.Errorf("position %d: got %q, want %q", index, sorted[index].Title, wantTitle)
		}
	}
}

func TestSort_CreatedDesc(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("old", now, now.Add(-72*time.Hour)),
		makePR("new", now, now.Add(-24*time.Hour)),
		makePR("mid", now, now.Add(-48*time.Hour)),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{By: query.SortCreated, Direction: query.SortDesc})

	if sorted[0].Title != "new" {
		t.Errorf("expected new first, got %q", sorted[0].Title)
	}
}

func TestSort_DoesNotMutateInput(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("B", now.Add(-48*time.Hour), now),
		makePR("A", now.Add(-24*time.Hour), now),
	}
	originalFirstTitle := pullRequests[0].Title

	query.Sort(pullRequests, query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc})

	if pullRequests[0].Title != originalFirstTitle {
		t.Error("Sort mutated the input slice")
	}
}

func TestSort_ZeroSpec(t *testing.T) {
	now := time.Now()
	pullRequests := []model.PullRequest{
		makePR("B", now, now),
		makePR("A", now, now),
	}

	sorted := query.Sort(pullRequests, query.SortSpec{})

	if sorted[0].Title != "B" || sorted[1].Title != "A" {
		t.Error("zero SortSpec should preserve original order")
	}
}
