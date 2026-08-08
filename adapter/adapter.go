package adapter

import (
	"context"

	"github.com/lstellway/prsm/model"
)

// RepoRef identifies a repository to poll within a provider.
// It lives here because every provider polls owner/repo pairs; provider-specific
// reference types belong in that provider's adapter package.
type RepoRef struct {
	Owner string
	Repo  string
}

// ProviderAdapter is the contract for all provider implementations.
// Each method corresponds to one layer of the lazy-fetch model from ADR-004.
type ProviderAdapter interface {
	// Kind returns the provider kind for this adapter instance.
	Kind() model.ProviderKind

	// Instance returns the full ProviderInstance this adapter serves.
	Instance() model.ProviderInstance

	// ListPullRequests fetches the current open PR list.
	// List-time fields on each PullRequest are populated;
	// lazy fields (CI, ReviewerStates, Diff) are LoadStatePending.
	ListPullRequests(ctx context.Context) ([]model.PullRequest, error)

	// LoadCI fetches CI status for a single PR and returns the updated value.
	LoadCI(ctx context.Context, pullRequest model.PullRequest) (model.CIStatus, error)

	// LoadReviewerStates fetches full reviewer decisions for a single PR.
	LoadReviewerStates(ctx context.Context, pullRequest model.PullRequest) ([]model.ReviewerState, error)

	// LoadDiff fetches commit and file-change counts for a single PR.
	LoadDiff(ctx context.Context, pullRequest model.PullRequest) (model.DiffStats, error)

	// ResolveIdentity returns the authenticated user's identity for this provider.
	// Called once at startup to resolve "me" sentinels in filters.
	// The result is the authenticated account, represented as a model.Identity.
	ResolveIdentity(ctx context.Context) (model.Identity, error)
}
