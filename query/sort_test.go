package query_test

import (
	"testing"
	"time"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func makePR(title string, updated, created time.Time) model.PullRequest {
	return model.PullRequest{Title: title, UpdatedAt: updated, CreatedAt: created}
}

func TestSort_UpdatedDesc(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("C", now.Add(-72*time.Hour), now.Add(-72*time.Hour)),
		makePR("A", now.Add(-24*time.Hour), now.Add(-24*time.Hour)),
		makePR("B", now.Add(-48*time.Hour), now.Add(-48*time.Hour)),
	}

	sorted := query.Sort(prs, query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc})

	want := []string{"A", "B", "C"}
	for i, w := range want {
		if sorted[i].Title != w {
			t.Errorf("position %d: got %q, want %q", i, sorted[i].Title, w)
		}
	}
}

func TestSort_UpdatedAsc(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("C", now.Add(-72*time.Hour), now),
		makePR("A", now.Add(-24*time.Hour), now),
		makePR("B", now.Add(-48*time.Hour), now),
	}

	sorted := query.Sort(prs, query.SortSpec{By: query.SortUpdated, Direction: query.SortAsc})

	want := []string{"C", "B", "A"}
	for i, w := range want {
		if sorted[i].Title != w {
			t.Errorf("position %d: got %q, want %q", i, sorted[i].Title, w)
		}
	}
}

func TestSort_Staleness(t *testing.T) {
	// Staleness = least recently updated first (oldest first)
	now := time.Now()
	prs := []model.PullRequest{
		makePR("recent", now.Add(-24*time.Hour), now),
		makePR("stale", now.Add(-168*time.Hour), now),
		makePR("mid", now.Add(-72*time.Hour), now),
	}

	sorted := query.Sort(prs, query.SortSpec{By: query.SortStaleness, Direction: query.SortAsc})

	if sorted[0].Title != "stale" {
		t.Errorf("expected stale first, got %q", sorted[0].Title)
	}
	if sorted[2].Title != "recent" {
		t.Errorf("expected recent last, got %q", sorted[2].Title)
	}
}

func TestSort_TitleAsc(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("Zebra fix", now, now),
		makePR("add feature", now, now),
		makePR("Bump version", now, now),
	}

	sorted := query.Sort(prs, query.SortSpec{By: query.SortTitle, Direction: query.SortAsc})

	want := []string{"add feature", "Bump version", "Zebra fix"}
	for i, w := range want {
		if sorted[i].Title != w {
			t.Errorf("position %d: got %q, want %q", i, sorted[i].Title, w)
		}
	}
}

func TestSort_CreatedDesc(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("old", now, now.Add(-72*time.Hour)),
		makePR("new", now, now.Add(-24*time.Hour)),
		makePR("mid", now, now.Add(-48*time.Hour)),
	}

	sorted := query.Sort(prs, query.SortSpec{By: query.SortCreated, Direction: query.SortDesc})

	if sorted[0].Title != "new" {
		t.Errorf("expected new first, got %q", sorted[0].Title)
	}
}

func TestSort_DoesNotMutateInput(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("B", now.Add(-48*time.Hour), now),
		makePR("A", now.Add(-24*time.Hour), now),
	}
	origFirst := prs[0].Title

	query.Sort(prs, query.SortSpec{By: query.SortUpdated, Direction: query.SortDesc})

	if prs[0].Title != origFirst {
		t.Error("Sort mutated the input slice")
	}
}

func TestSort_ZeroSpec(t *testing.T) {
	now := time.Now()
	prs := []model.PullRequest{
		makePR("B", now, now),
		makePR("A", now, now),
	}

	sorted := query.Sort(prs, query.SortSpec{})

	if sorted[0].Title != "B" || sorted[1].Title != "A" {
		t.Error("zero SortSpec should preserve original order")
	}
}
