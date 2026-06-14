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

// Sort returns a new slice sorted according to spec. The input is not modified.
// A zero SortSpec (By == "") returns a copy in the original order.
func Sort(prs []model.PullRequest, spec SortSpec) []model.PullRequest {
	result := make([]model.PullRequest, len(prs))
	copy(result, prs)

	if spec.By == "" {
		return result
	}

	sort.SliceStable(result, func(i, j int) bool {
		return less(result[i], result[j], spec)
	})

	return result
}

func less(a, b model.PullRequest, spec SortSpec) bool {
	var asc bool
	switch spec.By {
	case SortUpdated:
		asc = a.UpdatedAt.Before(b.UpdatedAt)
	case SortCreated:
		asc = a.CreatedAt.Before(b.CreatedAt)
	case SortStaleness:
		// Staleness = least recently updated first (ascending by UpdatedAt)
		asc = a.UpdatedAt.Before(b.UpdatedAt)
	case SortTitle:
		asc = strings.ToLower(a.Title) < strings.ToLower(b.Title)
	default:
		return false
	}

	if spec.Direction == SortDesc {
		return !asc
	}
	return asc
}
