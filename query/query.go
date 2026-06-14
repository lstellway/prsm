package query

import "github.com/lstellway/prsm/model"

// Result is the output of a full query pipeline pass.
type Result struct {
	Groups   []Group // at least one group; GroupNone produces a single unnamed group
	Total    int     // PR count before structural filter
	Filtered int     // PR count after structural filter, before fuzzy
	Shown    int     // PR count visible after fuzzy; equals Filtered when fuzzyQuery is empty
}

// Apply runs the standard query pipeline: structural filter → sort → fuzzy → group.
// A nil filter passes all PRs through. When fuzzyQuery is empty the sort order is
// preserved; when non-empty, results are re-ranked by fuzzy score, overriding sort order.
func Apply(
	prs []model.PullRequest,
	filter Predicate[model.PullRequest],
	sort SortSpec,
	group GroupSpec,
	fuzzyQuery string,
) Result {
	filtered := make([]model.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if filter == nil || filter(pr) {
			filtered = append(filtered, pr)
		}
	}

	sorted := Sort(filtered, sort)

	shown := sorted
	if fuzzyQuery != "" {
		shown = FuzzyMatch(sorted, fuzzyQuery)
	}

	return Result{
		Groups:   GroupBy(shown, group),
		Total:    len(prs),
		Filtered: len(filtered),
		Shown:    len(shown),
	}
}
