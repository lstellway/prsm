package query

import (
	"sort"
	"strings"

	"github.com/lstellway/prsm/model"
)

// FuzzyMatch returns the subset of pullRequests whose composite target string matches query
// using an fzf-style subsequence algorithm, ordered by descending match score.
// An empty query returns pullRequests unchanged.
func FuzzyMatch(pullRequests []model.PullRequest, query string) []model.PullRequest {
	if query == "" {
		return pullRequests
	}
	query = strings.ToLower(query)

	type scored struct {
		pullRequest model.PullRequest
		score       int
	}

	results := make([]scored, 0, len(pullRequests))
	for _, pullRequest := range pullRequests {
		target := buildFuzzyTarget(pullRequest)
		if score, ok := fuzzyScore(target, query); ok {
			results = append(results, scored{pullRequest, score})
		}
	}

	sort.SliceStable(results, func(leftIndex, rightIndex int) bool {
		return results[leftIndex].score > results[rightIndex].score
	})

	matched := make([]model.PullRequest, len(results))
	for index, scoredResult := range results {
		matched[index] = scoredResult.pullRequest
	}
	return matched
}

// buildFuzzyTarget constructs the composite lowercase string that fuzzy matching runs against.
// Composite field order follows ADR-006: Title, Author, Repo, Labels, Branches.
func buildFuzzyTarget(pullRequest model.PullRequest) string {
	parts := make([]string, 0, 8+len(pullRequest.Labels))
	parts = append(parts, pullRequest.Title, pullRequest.Author.Username)
	if pullRequest.Author.DisplayName != "" {
		parts = append(parts, pullRequest.Author.DisplayName)
	}
	parts = append(parts, pullRequest.Repo.Owner, pullRequest.Repo.Name)
	for _, label := range pullRequest.Labels {
		parts = append(parts, label.Name)
	}
	parts = append(parts, pullRequest.SourceBranch, pullRequest.TargetBranch)
	return strings.ToLower(strings.Join(parts, " "))
}

// fuzzyScore returns the match score and whether query is a subsequence of target.
// Both strings should already be lowercased. Scoring bonuses:
//   - +4 for consecutive character matches
//   - +3 for a match at a word boundary (after space, /, _, -, .)
//   - +2 for a match at the start of the target string
func fuzzyScore(target, query string) (score int, matched bool) {
	queryIndex := 0
	lastMatch := -2

	for targetIndex := 0; targetIndex < len(target) && queryIndex < len(query); targetIndex++ {
		if target[targetIndex] != query[queryIndex] {
			continue
		}
		charScore := 1
		if targetIndex == lastMatch+1 {
			charScore += 4
		}
		if targetIndex == 0 {
			charScore += 2
		}
		if targetIndex > 0 {
			previousChar := target[targetIndex-1]
			if previousChar == ' ' || previousChar == '/' || previousChar == '_' || previousChar == '-' || previousChar == '.' {
				charScore += 3
			}
		}
		score += charScore
		lastMatch = targetIndex
		queryIndex++
	}

	return score, queryIndex == len(query)
}
