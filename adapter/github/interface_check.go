package github

import "github.com/lstellway/prsm/adapter"

// Compile-time assertions: what a GitHubAdapter serves, stated as the set of
// adapter interfaces it satisfies. The assembly layer determines the same set
// by interface assertion at construction, so a method dropped or renamed here
// is a silently unserved resource kind rather than a compile error — these
// assertions are what turn it back into one.
var (
	_ adapter.Connection        = (*GitHubAdapter)(nil)
	_ adapter.PullRequestSource = (*GitHubAdapter)(nil)
	_ adapter.IdentityResolver  = (*GitHubAdapter)(nil)
)
