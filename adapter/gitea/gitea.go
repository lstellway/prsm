// Package gitea will implement prsm's provider adapter for Gitea, Forgejo, and
// Codeberg instances — one adapter covering all three, since Codeberg is a
// Forgejo instance and the two vendors' APIs have not diverged enough yet to
// need separate adapters. Only the vendor-local Config and RepoRef types exist
// today; the GiteaAdapter implementing adapter.PullRequestSource has not been
// built yet — see adapter/github for the reference shape this package will follow.
package gitea

// RepoRef identifies a repository to poll. Gitea inherits GitHub's owner/repo
// addressing, but scope types are vendor-local: the shape matching another
// vendor's today is not a reason to share one type, since Gitea and Forgejo are
// already drifting apart from each other.
type RepoRef struct {
	Owner string
	Repo  string
}

// Config holds the parameters needed to construct a GiteaAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
// Token takes precedence; Username+Password are only used when Token is empty.
type Config struct {
	Name     string
	Token    string
	BaseURL  string
	Repos    []RepoRef
	Username string
	Password string
}
