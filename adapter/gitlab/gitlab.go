// Package gitlab will implement prsm's provider adapter for GitLab.com and
// self-hosted GitLab. Only the vendor-local Config, ProjectRef, and GroupRef
// types exist today; the GitLabAdapter implementing adapter.PullRequestSource
// has not been built yet — see adapter/github for the reference shape this
// package will follow.
package gitlab

// ProjectRef identifies a GitLab project to poll. GitLab calls a repository a
// project, and scope types are vendor-local, so the type lives here rather than
// in the shared adapter package.
//
// The owner/name shape is inherited from the config layer's repo entries.
// Whether GitLab's real addressing is a namespace path rather than a pair is
// the GitLab adapter's decision to make when it is implemented; the type being
// local is what leaves that decision open.
type ProjectRef struct {
	Owner string
	Repo  string
}

// GroupRef identifies a GitLab group or namespace to poll. It is GitLab-specific,
// so it lives here rather than in the shared adapter package.
type GroupRef struct {
	Path string
}

// Config holds the parameters needed to construct a GitLabAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
type Config struct {
	Name     string
	Token    string
	BaseURL  string
	Projects []ProjectRef
	Groups   []GroupRef
}
