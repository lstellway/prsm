// Package model defines prsm's normalized resource types — PullRequest,
// Identity, ProviderInstance, and the lazy-fetch LoadResult[T] wrapper — the
// vendor-agnostic schema every adapter normalizes into and every consumer
// reads from. It carries no presentation or transport concerns and imports
// nothing else from this module.
package model

import "time"

// PRState is the lifecycle state of a PR.
type PRState string

const (
	// PRStateUnknown is the zero value: no lifecycle state has been assigned.
	// Every adapter sets State during normalization, so a PullRequest in a snapshot
	// should never hold it; the sentinel exists so a partial literal cannot pass
	// itself off as open.
	PRStateUnknown PRState = ""
	PRStateOpen    PRState = "open"
	PRStateClosed  PRState = "closed"
	PRStateMerged  PRState = "merged"
	PRStateDraft   PRState = "draft" // open + draft; surfaced separately for triage filtering
)

// IsKnown reports whether a lifecycle state has been assigned. False for the zero value.
func (prState PRState) IsKnown() bool {
	return prState != PRStateUnknown
}

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
	State PRState
	Draft bool // true when the PR is a draft; set alongside PRStateDraft for providers that expose both

	// Mergeable — availability varies by connection: GitLab and Gitea return it from
	// the list response, GitHub requires the detail endpoint. Pending means prsm has
	// not asked; Loaded(MergeableStateUnknown) means it asked and the provider is
	// still computing.
	Mergeable LoadResult[MergeableState]

	// Participants
	Author Author

	// Review
	Reviews ReviewSummary

	// CI — lazy-loaded; GitLab may populate from the list response
	CI LoadResult[CIStatus]

	// CommentCount is the general discussion comment count, excluding inline review
	// comments. Availability varies by connection — GitHub's list endpoint omits it —
	// so it is wrapped: a bare int could not distinguish "no comments" from
	// "not fetched", and unlike an enum it has no room for a sentinel.
	CommentCount LoadResult[int]

	// Diff — lazy-loaded from the PR detail endpoint
	Diff LoadResult[DiffStats]

	// Labels
	Labels []Label

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
	MergedAt  *time.Time // nil if not merged
}

// PullRequestRef identifies a pull request for routing a lazy-load call back
// to its connection, without carrying the full normalized PullRequest —
// content fields like Title, Body, Labels, and the LoadResult-wrapped fields
// are irrelevant to routing and are deliberately left off. HeadSHA is
// included because LoadCI needs it directly, not just to find the PR.
// ProviderID carries the same provider-scoped internal ID as
// PullRequest.ProviderID — GitHub's adapter has no use for it today
// (Repo/Number address a PR through the REST API it calls), but a vendor
// whose lazy-load endpoints address a PR by internal ID rather than
// owner/repo/number will need it here rather than reintroducing a full
// PullRequest just to carry one more field.
type PullRequestRef struct {
	Provider   ProviderInstance
	Repo       Repository
	Number     int
	ProviderID string
	HeadSHA    string
}

// Ref builds the routing handle for this PullRequest.
func (pullRequest PullRequest) Ref() PullRequestRef {
	return PullRequestRef{
		Provider:   pullRequest.Provider,
		Repo:       pullRequest.Repo,
		Number:     pullRequest.Number,
		ProviderID: pullRequest.ProviderID,
		HeadSHA:    pullRequest.HeadSHA,
	}
}
