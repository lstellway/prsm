package model

// ProviderKind identifies the git hosting software, independent of instance.
type ProviderKind string

const (
	ProviderGitHub ProviderKind = "github"
	ProviderGitLab ProviderKind = "gitlab"
	ProviderGitea  ProviderKind = "gitea" // covers Gitea, Forgejo, and Codeberg
)

// ProviderInstance identifies one configured account/server combination.
// Multiple instances of the same kind are supported (e.g., github.com and github.example.com).
type ProviderInstance struct {
	Name    string       // user-assigned config alias, e.g., "github-personal"; matches provider filter in views
	Kind    ProviderKind
	Host    string // canonical hostname, e.g., "github.com" or "gitlab.example.com"
	Account string // username or org slug used for this credential
}
