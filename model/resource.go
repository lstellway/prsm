package model

// ResourceKind identifies a kind of resource a connection serves. A vendor
// serves some kinds and not others: Jenkins and CircleCI have CI runs but no
// pull requests, and the local git checkout has branches and worktrees but no
// host and no credential. Which kinds a given connection serves is therefore a
// per-connection fact, not a property of the vendor.
//
// The set is closed. A kind is added here alongside its normalized model type
// and its adapter interface, never by a consumer inventing a string.
type ResourceKind string

const (
	// ResourceKindUnknown is the zero value: no kind has been assigned. Like
	// ProviderUnknown this is a validation signal rather than a lifecycle state —
	// nothing legitimately carries it, and the layer that failed to set it is
	// the bug.
	ResourceKindUnknown ResourceKind = ""

	// ResourceKindPullRequest is the v1 resource kind. Its value matches the
	// `resource = "pr"` config vocabulary so the two never diverge.
	ResourceKindPullRequest ResourceKind = "pr"
)

// IsKnown reports whether a resource kind has been assigned. False for the zero value.
func (resourceKind ResourceKind) IsKnown() bool {
	return resourceKind != ResourceKindUnknown
}
