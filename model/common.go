package model

// Identity is the normalized identity of a person on a provider.
// DisplayName may be empty if the provider does not supply one; fall back to Username.
type Identity struct {
	Username    string // provider-scoped login; not unique across providers
	DisplayName string // may be empty or redacted depending on provider/permissions
	AvatarURL   string // empty if not available
}

// Author is an Identity in the context of PR authorship.
type Author = Identity

// Reviewer is an Identity in the context of PR review participation.
type Reviewer = Identity

// Label is a normalized tag attached to a PR.
type Label struct {
	Name  string
	Color string // hex color, e.g., "#0075ca"; empty if the provider does not supply one
}

// Repository identifies the repository a PR belongs to.
type Repository struct {
	Owner string // org or user namespace
	Name  string // repository name without owner prefix
}

// MergeableState is the mergeability of a PR as reported by the provider.
type MergeableState string

const (
	MergeableStateUnknown     MergeableState = "" // zero value; not yet computed by provider
	MergeableStateMergeable   MergeableState = "mergeable"
	MergeableStateConflicting MergeableState = "conflicting"
)
