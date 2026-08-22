package model

// ProviderKind identifies the git hosting software, independent of instance.
type ProviderKind string

const (
	// ProviderUnknown is the zero value: no kind has been assigned. Unlike the
	// other Unknown sentinels this one is a validation signal, not a lifecycle
	// state — a ProviderInstance carried in a snapshot should never hold it, and
	// the assembly layer rejects it at adapter construction rather than propagating it.
	ProviderUnknown ProviderKind = ""
	ProviderGitHub  ProviderKind = "github"
	ProviderGitLab  ProviderKind = "gitlab"
	ProviderGitea   ProviderKind = "gitea" // covers Gitea, Forgejo, and Codeberg
)

// IsKnown reports whether a provider kind has been assigned. False for the zero value.
func (providerKind ProviderKind) IsKnown() bool {
	return providerKind != ProviderUnknown
}

// KnownProviderKinds returns every ProviderKind this codebase recognizes, in
// a fixed display order. config.LoadFile's enum validation reads from here
// instead of maintaining its own copy of the vendor list; prsm.
// constructConnection switches on the same named constants directly.
func KnownProviderKinds() []ProviderKind {
	return []ProviderKind{ProviderGitHub, ProviderGitLab, ProviderGitea}
}

// ProviderInstance identifies one configured account/server combination.
// Multiple instances of the same kind are supported (e.g., github.com and github.example.com).
type ProviderInstance struct {
	Name    string // user-assigned config alias, e.g., "github-personal"; matches provider filter in views
	Kind    ProviderKind
	Host    string // canonical hostname, e.g., "github.com" or "gitlab.example.com"
	Account string // username or org slug used for this credential
}
