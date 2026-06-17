package gitea

import "github.com/lstellway/prsm/adapter"

// Config holds the parameters needed to construct a GiteaAdapter.
// The assembly layer maps config.ProviderConfig into this type so the adapter
// package has no dependency on the config package.
// Token takes precedence; Username+Password are only used when Token is empty.
type Config struct {
	Name     string
	Token    string
	BaseURL  string
	Repos    []adapter.RepoRef
	Username string
	Password string
}
