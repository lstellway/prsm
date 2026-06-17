package gitlab

import "github.com/lstellway/prsm/adapter"

// Config holds the parameters needed to construct a GitLabAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
type Config struct {
	Name    string
	Token   string
	BaseURL string
	Repos   []adapter.RepoRef
	Groups  []adapter.GroupRef
}
