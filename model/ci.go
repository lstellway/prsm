package model

// CIState is the overall CI/check-run status for the PR head commit.
type CIState string

const (
	CIStatePassing CIState = "passing"
	CIStateFailing CIState = "failing"
	CIStatePending CIState = "pending"
	CIStateNone    CIState = "none" // no CI configured or checks not found
)

// CIStatus holds the overall CI result for the PR's head commit.
// Wrapped in LoadResult because availability and fetch cost vary by provider.
type CIStatus struct {
	State   CIState
	Summary string // constructed by the adapter, e.g., "3 checks passed, 1 failed"
	URL     string // link to the CI run; empty if the provider does not supply one
}
