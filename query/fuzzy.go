package query

import (
	"sort"
	"strings"

	"github.com/lstellway/prsm/model"
)

// FuzzyMatch returns the subset of prs whose composite target string matches query
// using an fzf-style subsequence algorithm, ordered by descending match score.
// An empty query returns prs unchanged.
func FuzzyMatch(prs []model.PullRequest, query string) []model.PullRequest {
	if query == "" {
		return prs
	}
	query = strings.ToLower(query)

	type scored struct {
		pr    model.PullRequest
		score int
	}

	results := make([]scored, 0, len(prs))
	for _, pr := range prs {
		target := buildFuzzyTarget(pr)
		if score, ok := fuzzyScore(target, query); ok {
			results = append(results, scored{pr, score})
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	out := make([]model.PullRequest, len(results))
	for i, r := range results {
		out[i] = r.pr
	}
	return out
}

// buildFuzzyTarget constructs the composite lowercase string that fuzzy matching runs against.
// Composite field order follows ADR-006: Title, Author, Repo, Labels, Branches.
func buildFuzzyTarget(pr model.PullRequest) string {
	parts := make([]string, 0, 8+len(pr.Labels))
	parts = append(parts, pr.Title, pr.Author.Username)
	if pr.Author.DisplayName != "" {
		parts = append(parts, pr.Author.DisplayName)
	}
	parts = append(parts, pr.Repo.Owner, pr.Repo.Name)
	for _, l := range pr.Labels {
		parts = append(parts, l.Name)
	}
	parts = append(parts, pr.SourceBranch, pr.TargetBranch)
	return strings.ToLower(strings.Join(parts, " "))
}

// fuzzyScore returns the match score and whether query is a subsequence of target.
// Both strings should already be lowercased. Scoring bonuses:
//   - +4 for consecutive character matches
//   - +3 for a match at a word boundary (after space, /, _, -, .)
//   - +2 for a match at the start of the target string
func fuzzyScore(target, query string) (score int, matched bool) {
	qi := 0
	lastMatch := -2

	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] != query[qi] {
			continue
		}
		charScore := 1
		if ti == lastMatch+1 {
			charScore += 4
		}
		if ti == 0 {
			charScore += 2
		}
		if ti > 0 {
			prev := target[ti-1]
			if prev == ' ' || prev == '/' || prev == '_' || prev == '-' || prev == '.' {
				charScore += 3
			}
		}
		score += charScore
		lastMatch = ti
		qi++
	}

	return score, qi == len(query)
}
