package query

import (
	"sort"

	"github.com/lstellway/prsm/model"
)

// GroupKey identifies the field to group by.
type GroupKey string

const (
	GroupNone         GroupKey = "none"          // universal: flat list, no grouping
	GroupRepo         GroupKey = "repo"           // universal: by Repo.Owner + "/" + Repo.Name
	GroupProvider     GroupKey = "provider"       // universal: by Provider.Account
	GroupAuthor       GroupKey = "author"         // universal: by Author.Username
	GroupReviewStatus GroupKey = "review_status"  // PR-only: by Reviews.AggregateState
)

// GroupSpec describes a group operation.
type GroupSpec struct {
	By GroupKey
}

// ValidateForResource returns an error if the GroupKey is not valid for the given resource type.
// resourceType should be "pr" for pull requests. PR-only keys are invalid for other resource types.
func (s GroupSpec) ValidateForResource(resourceType string) error {
	if resourceType != "pr" && s.By == GroupReviewStatus {
		return &groupKeyError{key: s.By, resource: resourceType}
	}
	return nil
}

type groupKeyError struct {
	key      GroupKey
	resource string
}

func (e *groupKeyError) Error() string {
	return "group key \"" + string(e.key) + "\" is only valid for resource type \"pr\", not \"" + e.resource + "\""
}

// Group holds the result of grouping a set of pull requests under a single key.
type Group struct {
	Key   string
	Items []model.PullRequest
}

// GroupBy partitions prs into named groups according to spec, with each group's items in
// their original relative order. Groups themselves are ordered per ADR-006:
//   - repo/provider: alphabetical by key
//   - author: descending by PR count
//   - review_status: triage priority (review_required → changes_requested → commented → approved → none)
//   - none: single group with all items
func GroupBy(prs []model.PullRequest, spec GroupSpec) []Group {
	if spec.By == GroupNone || spec.By == "" {
		return []Group{{Key: "", Items: prs}}
	}

	index := make(map[string][]model.PullRequest)
	order := make([]string, 0)

	for _, pr := range prs {
		key := groupKey(pr, spec.By)
		if _, seen := index[key]; !seen {
			order = append(order, key)
		}
		index[key] = append(index[key], pr)
	}

	groups := make([]Group, 0, len(index))
	for _, k := range order {
		groups = append(groups, Group{Key: k, Items: index[k]})
	}

	sortGroups(groups, spec.By)
	return groups
}

func groupKey(pr model.PullRequest, by GroupKey) string {
	switch by {
	case GroupRepo:
		return pr.Repo.Owner + "/" + pr.Repo.Name
	case GroupProvider:
		return pr.Provider.Account
	case GroupAuthor:
		return pr.Author.Username
	case GroupReviewStatus:
		s := string(pr.Reviews.AggregateState)
		if s == "" {
			return "none"
		}
		return s
	default:
		return ""
	}
}

func sortGroups(groups []Group, by GroupKey) {
	switch by {
	case GroupRepo, GroupProvider:
		sort.SliceStable(groups, func(i, j int) bool {
			return groups[i].Key < groups[j].Key
		})
	case GroupAuthor:
		sort.SliceStable(groups, func(i, j int) bool {
			return len(groups[i].Items) > len(groups[j].Items)
		})
	case GroupReviewStatus:
		sort.SliceStable(groups, func(i, j int) bool {
			return reviewStatusPriority(groups[i].Key) < reviewStatusPriority(groups[j].Key)
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
	case "none":
		return 4
	default:
		return 5
	}
}
