package query

import (
	"sort"

	"github.com/lstellway/prsm/model"
)

// GroupKey identifies the field to group by.
type GroupKey string

const (
	GroupNone         GroupKey = "none"          // universal: flat list, no grouping
	GroupRepo         GroupKey = "repo"          // universal: by Repo.Owner + "/" + Repo.Name
	GroupProvider     GroupKey = "provider"      // universal: by Provider.Account
	GroupAuthor       GroupKey = "author"        // universal: by Author.Username
	GroupReviewStatus GroupKey = "review_status" // PR-only: by Reviews.AggregateState
)

// groupKeyUnknown is the bucket for items whose group field has no value yet.
// It is prsm's own bookkeeping surfacing in the view, not a state any provider reports.
const groupKeyUnknown = "unknown"

// GroupSpec describes a group operation.
type GroupSpec struct {
	By GroupKey
}

// ValidateForResource returns an error if the GroupKey is not valid for the given resource type.
// resourceType should be "pr" for pull requests. PR-only keys are invalid for other resource types.
func (groupSpec GroupSpec) ValidateForResource(resourceType string) error {
	if resourceType != "pr" && groupSpec.By == GroupReviewStatus {
		return &groupKeyError{key: groupSpec.By, resource: resourceType}
	}
	return nil
}

type groupKeyError struct {
	key      GroupKey
	resource string
}

func (groupKeyErr *groupKeyError) Error() string {
	return "group key \"" + string(groupKeyErr.key) + "\" is only valid for resource type \"pr\", not \"" + groupKeyErr.resource + "\""
}

// Group holds the result of grouping a set of pull requests under a single key.
type Group struct {
	Key   string
	Items []model.PullRequest
}

// GroupBy partitions pullRequests into named groups according to groupSpec, with each group's
// items in their original relative order. Groups themselves are ordered:
//   - repo/provider: alphabetical by key
//   - author: descending by PR count
//   - review_status: triage priority (review_required → changes_requested → commented → approved → none)
//   - none: single group with all items
func GroupBy(pullRequests []model.PullRequest, groupSpec GroupSpec) []Group {
	if groupSpec.By == GroupNone || groupSpec.By == "" {
		return []Group{{Key: "", Items: pullRequests}}
	}

	index := make(map[string][]model.PullRequest)
	order := make([]string, 0)

	for _, pullRequest := range pullRequests {
		key := groupKey(pullRequest, groupSpec.By)
		if _, seen := index[key]; !seen {
			order = append(order, key)
		}
		index[key] = append(index[key], pullRequest)
	}

	groups := make([]Group, 0, len(index))
	for _, key := range order {
		groups = append(groups, Group{Key: key, Items: index[key]})
	}

	sortGroups(groups, groupSpec.By)
	return groups
}

func groupKey(pullRequest model.PullRequest, groupBy GroupKey) string {
	switch groupBy {
	case GroupRepo:
		return pullRequest.Repo.Owner + "/" + pullRequest.Repo.Name
	case GroupProvider:
		return pullRequest.Provider.Account
	case GroupAuthor:
		return pullRequest.Author.Username
	case GroupReviewStatus:
		// An uncomputed aggregate gets its own bucket. Folding it into "none" would
		// tell the reader a PR has no reviews when prsm has not yet looked.
		if !pullRequest.Reviews.AggregateState.IsKnown() {
			return groupKeyUnknown
		}
		return string(pullRequest.Reviews.AggregateState)
	default:
		return ""
	}
}

func sortGroups(groups []Group, groupBy GroupKey) {
	switch groupBy {
	case GroupRepo, GroupProvider:
		sort.SliceStable(groups, func(leftIndex, rightIndex int) bool {
			return groups[leftIndex].Key < groups[rightIndex].Key
		})
	case GroupAuthor:
		sort.SliceStable(groups, func(leftIndex, rightIndex int) bool {
			return len(groups[leftIndex].Items) > len(groups[rightIndex].Items)
		})
	case GroupReviewStatus:
		sort.SliceStable(groups, func(leftIndex, rightIndex int) bool {
			return reviewStatusPriority(groups[leftIndex].Key) < reviewStatusPriority(groups[rightIndex].Key)
		})
	}
}

// reviewStatusPriority returns the triage sort order for review_status group keys.
// Lower number = higher priority (appears first).
func reviewStatusPriority(key string) int {
	switch key {
	case string(model.AggregateReviewStateRequired):
		return 0
	case string(model.AggregateReviewStateChangesRequested):
		return 1
	case string(model.AggregateReviewStateCommented):
		return 2
	case string(model.AggregateReviewStateApproved):
		return 3
	case string(model.AggregateReviewStateNone):
		return 4
	case groupKeyUnknown:
		return 5
	default:
		return 6
	}
}
