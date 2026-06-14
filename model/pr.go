package model

import "time"

// PRState is the lifecycle state of a PR.
type PRState string

const (
	PRStateOpen   PRState = "open"
	PRStateClosed PRState = "closed"
	PRStateMerged PRState = "merged"
	PRStateDraft  PRState = "draft" // open + draft; surfaced separately for triage filtering
)

// PullRequest is the normalized internal representation of a pull request or merge request
// from any v1 provider. All display, filtering, sorting, and grouping operates on this type.
type PullRequest struct {
	// Identity
	ProviderID string // provider-scoped internal ID stored as string for provider independence
	Number     int    // human-visible PR/MR number, e.g., 1234
	Provider   ProviderInstance
	Repo       Repository
	URL        string

	// Content
	Title        string
	SourceBranch string // head branch being merged
	TargetBranch string // base branch being merged into
	HeadSHA      string // tip commit SHA; used by the Event Engine to detect new commits
	Body         string // PR description; may be empty

	// State
	State     PRState
	Mergeable MergeableState // zero value is MergeableStateUnknown

	// Participants
	Author Author

	// Review
	Reviews ReviewSummary

	// CI — lazy-loaded; GitLab may populate from the list response
	CI LoadResult[CIStatus]

	// CommentCount is the general discussion comment count, eagerly loaded from the list response.
	// Does not include inline review comments.
	CommentCount int

	// Diff — lazy-loaded from the PR detail endpoint
	Diff LoadResult[DiffStats]

	// Labels
	Labels []Label

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
	MergedAt  *time.Time // nil if not merged
}
