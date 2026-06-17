package model

// ReviewDecision is one reviewer's verdict on a PR.
// Maps the union of GitHub, GitLab, and Gitea review state strings.
type ReviewDecision string

const (
	ReviewDecisionApproved         ReviewDecision = "approved"
	ReviewDecisionChangesRequested ReviewDecision = "changes_requested"
	ReviewDecisionCommented        ReviewDecision = "commented"
	ReviewDecisionDismissed        ReviewDecision = "dismissed"
	ReviewDecisionPending          ReviewDecision = "pending" // requested but no decision submitted yet
)

// AggregateReviewState is the rolled-up review verdict for display and triage sorting.
type AggregateReviewState string

const (
	AggregateReviewStateNone             AggregateReviewState = "" // zero value; no reviewers requested or assigned
	AggregateReviewStateRequired         AggregateReviewState = "review_required"
	AggregateReviewStateApproved         AggregateReviewState = "approved"
	AggregateReviewStateChangesRequested AggregateReviewState = "changes_requested"
	AggregateReviewStateCommented        AggregateReviewState = "commented"
)

// ReviewerState captures one reviewer's identity and their current decision.
type ReviewerState struct {
	Reviewer Reviewer
	Decision ReviewDecision
}

// ComputeAggregateReviewState derives the rolled-up verdict from a set of loaded
// reviewer decisions. Priority: ChangesRequested > Pending > Approved > Commented.
// Called by consumers after a LoadReviewerStates call resolves.
func ComputeAggregateReviewState(states []ReviewerState) AggregateReviewState {
	if len(states) == 0 {
		return AggregateReviewStateNone
	}

	var hasChangesRequested, hasApproved, hasCommented, hasPending bool
	for _, s := range states {
		switch s.Decision {
		case ReviewDecisionChangesRequested:
			hasChangesRequested = true
		case ReviewDecisionApproved:
			hasApproved = true
		case ReviewDecisionCommented:
			hasCommented = true
		case ReviewDecisionPending:
			hasPending = true
		}
	}

	switch {
	case hasChangesRequested:
		return AggregateReviewStateChangesRequested
	case hasPending:
		return AggregateReviewStateRequired
	case hasApproved:
		return AggregateReviewStateApproved
	case hasCommented:
		return AggregateReviewStateCommented
	default:
		return AggregateReviewStateNone
	}
}

// ReviewSummary is the full review picture for a PR.
type ReviewSummary struct {
	// RequestedReviewers is populated from the list API for providers that return it.
	// Contains reviewer identities with a Pending decision.
	RequestedReviewers []ReviewerState

	// ReviewerStates contains submitted review decisions per reviewer.
	// Lazy-loaded via a secondary /reviews call (GitHub, Gitea) or /approvals (GitLab).
	ReviewerStates LoadResult[[]ReviewerState]

	// AggregateState is computed after ReviewerStates loads; before that it is
	// derived from RequestedReviewers alone (any requested reviewer → AggregateReviewStateRequired).
	AggregateState AggregateReviewState
}
