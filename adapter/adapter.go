// Package adapter defines the contracts a vendor implementation satisfies and
// the error types every implementation shares.
//
// The contracts are split along the resource-kind axis rather than gathered
// into one interface. A vendor serves some resource kinds and not others, so
// one interface listing every method would force each implementation to answer
// for kinds it does not serve — as a stub returning an error, which reports a
// permanent structural fact as a runtime failure. Interface assertion reports
// the same fact at construction, with no stub to write.
//
// Scope types are vendor-local. There is no shared repository reference here:
// scope is vendor vocabulary — owner/repo pairs on GitHub, project and group
// paths on GitLab, job paths on Jenkins, a filesystem root on a local checkout —
// and no single type spans it. See each vendor package for its own.
package adapter

import (
	"context"

	"github.com/lstellway/prsm/model"
)

// Connection is the contract every source satisfies: it can name the configured
// instance it serves. It requires nothing else. A source may have no host and no
// credential — the local git checkout has neither — and must still be
// representable, so anything beyond identifying itself belongs on an interface a
// connection may decline to implement.
//
// What a connection serves is expressed by the resource-kind interfaces below,
// which the assembly layer determines by interface assertion at construction. A
// connection implementing none of them is legal; it serves nothing.
type Connection interface {
	// Instance returns the ProviderInstance this connection serves.
	Instance() model.ProviderInstance
}

// PullRequestSource is implemented by connections serving
// model.ResourceKindPullRequest. Each method corresponds to one layer of the
// lazy-fetch model.
type PullRequestSource interface {
	Connection

	// ListPullRequests fetches the current open PR list.
	// List-time fields on each PullRequest are populated;
	// lazy fields (CI, ReviewerStates, Diff) are LoadStatePending.
	ListPullRequests(ctx context.Context) ([]model.PullRequest, error)

	// LoadCI fetches CI status for a single PR and returns the updated value.
	LoadCI(ctx context.Context, pullRequestRef model.PullRequestRef) (model.CIStatus, error)

	// LoadReviewerStates fetches full reviewer decisions for a single PR.
	LoadReviewerStates(ctx context.Context, pullRequestRef model.PullRequestRef) ([]model.ReviewerState, error)

	// LoadDiff fetches commit and file-change counts for a single PR.
	LoadDiff(ctx context.Context, pullRequestRef model.PullRequestRef) (model.DiffStats, error)
}

// IdentityResolver is implemented by connections that authenticate as somebody.
// It is optional: a credential-less source has no principal to report, and
// requiring it would make such a source structurally unrepresentable.
//
// Not implementing this interface is not a failure. Three states stay distinct
// for the assembly layer, and collapsing the first two would render a local
// checkout as a permanently broken connection:
//
//   - not implemented — no credential, no principal; healthy, contributes no
//     "me" identity
//   - implemented, call failed — the credential is bad, expired, or revoked
//   - implemented, resolved — contributes the "me" identity for this instance
type IdentityResolver interface {
	// ResolveIdentity returns the authenticated user's identity for this connection.
	// Called once at startup to resolve "me" sentinels in filters.
	// The result is the authenticated account, represented as a model.Identity.
	ResolveIdentity(ctx context.Context) (model.Identity, error)
}
