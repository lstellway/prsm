package query_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func pr(title string) model.PullRequest {
	return model.PullRequest{Title: title}
}

func prWithAll(title, author, owner, repo string, labels ...string) model.PullRequest {
	p := model.PullRequest{
		Title:  title,
		Author: model.Author{Username: author},
		Repo:   model.Repository{Owner: owner, Name: repo},
	}
	for _, l := range labels {
		p.Labels = append(p.Labels, model.Label{Name: l})
	}
	return p
}

func titles(prs []model.PullRequest) []string {
	out := make([]string, len(prs))
	for i, p := range prs {
		out[i] = p.Title
	}
	return out
}

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	prs := []model.PullRequest{pr("alpha"), pr("beta")}
	got := query.FuzzyMatch(prs, "")
	if len(got) != 2 {
		t.Errorf("empty query should return all PRs, got %d", len(got))
	}
}

func TestFuzzyMatch_ExactPrefix(t *testing.T) {
	prs := []model.PullRequest{
		pr("fix authentication bug"),
		pr("add feature flags"),
		pr("update readme"),
	}

	got := query.FuzzyMatch(prs, "fix")
	if len(got) != 1 {
		t.Fatalf("expected 1 match for 'fix', got %d: %v", len(got), titles(got))
	}
	if got[0].Title != "fix authentication bug" {
		t.Errorf("expected 'fix authentication bug', got %q", got[0].Title)
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	prs := []model.PullRequest{
		pr("fix authentication bug"),
		pr("add feature flags"),
	}

	got := query.FuzzyMatch(prs, "xyz")
	if len(got) != 0 {
		t.Errorf("expected 0 matches for 'xyz', got %d", len(got))
	}
}

func TestFuzzyMatch_MatchesAuthorAndLabel(t *testing.T) {
	prs := []model.PullRequest{
		prWithAll("some PR", "loganstellway", "acme", "api"),
		prWithAll("other PR", "alice", "acme", "api", "bugfix"),
		prWithAll("third PR", "bob", "acme", "api"),
	}

	// "logan" should match the first PR by author
	got := query.FuzzyMatch(prs, "logan")
	if len(got) != 1 {
		t.Fatalf("expected 1 match for 'logan', got %d", len(got))
	}
	if got[0].Title != "some PR" {
		t.Errorf("expected 'some PR', got %q", got[0].Title)
	}

	// "bugfix" should match the second PR by label
	got = query.FuzzyMatch(prs, "bugfix")
	if len(got) != 1 {
		t.Fatalf("expected 1 match for 'bugfix', got %d", len(got))
	}
	if got[0].Title != "other PR" {
		t.Errorf("expected 'other PR', got %q", got[0].Title)
	}
}

func TestFuzzyMatch_HigherScoreRanksFirst(t *testing.T) {
	// "auth" should score higher against "auth service fix" (prefix match)
	// than against "something with auth somewhere"
	prs := []model.PullRequest{
		pr("handle auth somewhere in code"),
		pr("auth service fix"),
	}

	got := query.FuzzyMatch(prs, "auth")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Title != "auth service fix" {
		t.Errorf("expected 'auth service fix' to rank first (higher score), got %q", got[0].Title)
	}
}

func TestFuzzyMatch_SubsequenceMatch(t *testing.T) {
	// "flg" matches "add feature flags" as subsequence: f(feature) l g (flags)
	// But does not match "update readme"
	prs := []model.PullRequest{
		pr("add feature flags"),
		pr("update readme"),
	}

	got := query.FuzzyMatch(prs, "flg")
	if len(got) == 0 {
		t.Fatal("expected 'flg' to match 'add feature flags' as subsequence")
	}
	if got[0].Title != "add feature flags" {
		t.Errorf("expected 'add feature flags', got %q", got[0].Title)
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	prs := []model.PullRequest{pr("Fix Authentication Bug")}

	got := query.FuzzyMatch(prs, "fix")
	if len(got) != 1 {
		t.Errorf("fuzzy match should be case-insensitive, got %d matches", len(got))
	}

	got = query.FuzzyMatch(prs, "FIX")
	if len(got) != 1 {
		t.Errorf("fuzzy match should be case-insensitive for uppercase query, got %d matches", len(got))
	}
}
