package query

import (
	"sort"
	"strings"

	"github.com/lstellway/prsm/model"
)

// SortKey identifies the field to sort by.
type SortKey string

const (
	SortUpdated   SortKey = "updated"   // by UpdatedAt
	SortCreated   SortKey = "created"   // by CreatedAt
	SortStaleness SortKey = "staleness" // by UpdatedAt ascending (least recently touched first)
	SortTitle     SortKey = "title"     // alphabetical by Title
)

// SortDirection controls ascending vs. descending order.
type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

// SortSpec describes a sort operation.
type SortSpec struct {
	By        SortKey
	Direction SortDirection
}

// Sort returns a new slice sorted according to sortSpec. The input is not modified.
// A zero SortSpec (By == "") returns a copy in the original order.
func Sort(pullRequests []model.PullRequest, sortSpec SortSpec) []model.PullRequest {
	result := make([]model.PullRequest, len(pullRequests))
	copy(result, pullRequests)

	if sortSpec.By == "" {
		return result
	}

	sort.SliceStable(result, func(leftIndex, rightIndex int) bool {
		return less(result[leftIndex], result[rightIndex], sortSpec)
	})

	return result
}

func less(left, right model.PullRequest, sortSpec SortSpec) bool {
	var ascending bool
	switch sortSpec.By {
	case SortUpdated:
		ascending = left.UpdatedAt.Before(right.UpdatedAt)
	case SortCreated:
		ascending = left.CreatedAt.Before(right.CreatedAt)
	case SortStaleness:
		// Staleness = days since UpdatedAt. Ascending = smallest staleness first (recently updated).
		// Direction is intentionally inverted relative to SortUpdated so that desc = most stale first,
		// matching the user's mental model of "staleness" as a magnitude (higher = more stale).
		ascending = left.UpdatedAt.After(right.UpdatedAt)
	case SortTitle:
		ascending = strings.ToLower(left.Title) < strings.ToLower(right.Title)
	default:
		return false
	}

	if sortSpec.Direction == SortDesc {
		return !ascending
	}
	return ascending
}
