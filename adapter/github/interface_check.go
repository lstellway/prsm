package github

import "github.com/lstellway/prsm/adapter"

// Compile-time assertion: GitHubAdapter must satisfy adapter.ProviderAdapter.
var _ adapter.ProviderAdapter = (*GitHubAdapter)(nil)
