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
	pullRequests []model.PullRequest,
	filter Predicate[model.PullRequest],
	sortSpec SortSpec,
	groupSpec GroupSpec,
	fuzzyQuery string,
) Result {
	filtered := make([]model.PullRequest, 0, len(pullRequests))
	for _, pullRequest := range pullRequests {
		if filter == nil || filter(pullRequest) {
			filtered = append(filtered, pullRequest)
		}
	}

	sorted := Sort(filtered, sortSpec)

	shown := sorted
	if fuzzyQuery != "" {
		shown = FuzzyMatch(sorted, fuzzyQuery)
	}

	return Result{
		Groups:   GroupBy(shown, groupSpec),
		Total:    len(pullRequests),
		Filtered: len(filtered),
		Shown:    len(shown),
	}
}
