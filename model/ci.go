package model

// CIState is the overall CI/check-run status for the PR head commit.
type CIState string

const (
	// CIStateUnknown is the zero value: no CI verdict has been established here.
	// Distinct from CIStateNone, which is the answer "CI ran nowhere on this PR".
	// A CIStatus reached through a Loaded LoadResult always carries a known state;
	// this sentinel exists so a bare CIStatus literal cannot claim otherwise.
	CIStateUnknown CIState = ""
	CIStatePassing CIState = "passing"
	CIStateFailing CIState = "failing"
	CIStatePending CIState = "pending"
	CIStateNone    CIState = "none" // no CI configured or checks not found
)

// IsKnown reports whether a CI verdict has been established. False for the zero value.
func (ciState CIState) IsKnown() bool {
	return ciState != CIStateUnknown
}

// CIStatus holds the overall CI result for the PR's head commit.
// Wrapped in LoadResult because availability and fetch cost vary by provider.
type CIStatus struct {
	State   CIState
	Summary string // constructed by the adapter, e.g., "3 checks passed, 1 failed"
	URL     string // link to the CI run; empty if the provider does not supply one
}
