package gitlab

import "github.com/lstellway/prsm/adapter"

// GroupRef identifies a GitLab group or namespace to poll. It is GitLab-specific,
// so it lives here rather than in the shared adapter package.
type GroupRef struct {
	Path string
}

// Config holds the parameters needed to construct a GitLabAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
type Config struct {
	Name    string
	Token   string
	BaseURL string
	Repos   []adapter.RepoRef
	Groups  []GroupRef
}
