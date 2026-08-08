package model

// ReviewDecision is one reviewer's verdict on a PR.
// Maps the union of GitHub, GitLab, and Gitea review state strings.
type ReviewDecision string

const (
	// ReviewDecisionUnknown is the zero value: no decision has been recorded here,
	// or the provider reported a state this adapter could not map. It is not a
	// verdict — every real verdict below has a non-empty value.
	ReviewDecisionUnknown          ReviewDecision = ""
	ReviewDecisionApproved         ReviewDecision = "approved"
	ReviewDecisionChangesRequested ReviewDecision = "changes_requested"
	ReviewDecisionCommented        ReviewDecision = "commented"
	ReviewDecisionDismissed        ReviewDecision = "dismissed"
	ReviewDecisionPending          ReviewDecision = "pending" // requested but no decision submitted yet
)

// IsKnown reports whether a decision has been recorded. False for the zero value.
func (reviewDecision ReviewDecision) IsKnown() bool {
	return reviewDecision != ReviewDecisionUnknown
}

// AggregateReviewState is the rolled-up review verdict for display and triage sorting.
type AggregateReviewState string

const (
	// AggregateReviewStateUnknown is the zero value: the aggregate has not been
	// computed for this PR. Distinct from AggregateReviewStateNone, which is the
	// computed answer "there are no reviews". A composite literal that never
	// assigns the field is therefore unknown rather than claiming an answer.
	AggregateReviewStateUnknown          AggregateReviewState = ""
	AggregateReviewStateNone             AggregateReviewState = "none"
	AggregateReviewStateRequired         AggregateReviewState = "review_required"
	AggregateReviewStateApproved         AggregateReviewState = "approved"
	AggregateReviewStateChangesRequested AggregateReviewState = "changes_requested"
	AggregateReviewStateCommented        AggregateReviewState = "commented"
)

// IsKnown reports whether the aggregate has been computed. False for the zero value.
func (aggregateReviewState AggregateReviewState) IsKnown() bool {
	return aggregateReviewState != AggregateReviewStateUnknown
}

// ReviewerState captures one reviewer's identity and their current decision.
type ReviewerState struct {
	Reviewer Reviewer
	Decision ReviewDecision
}

// ComputeAggregateReviewState derives the rolled-up verdict from a set of loaded
// reviewer decisions. Priority: ChangesRequested > Pending > Approved > Commented.
// Called by consumers after a LoadReviewerStates call resolves.
//
// The function is total: it returns a correct answer for any input, including nil
// (AggregateReviewStateNone — computed, and there are no reviews). It never returns
// AggregateReviewStateUnknown, because having been called is what makes the result
// known. This is why AggregateState is not wrapped in LoadResult — see ADR-004
// §Design notes → Unknown values.
func ComputeAggregateReviewState(states []ReviewerState) AggregateReviewState {
	if len(states) == 0 {
		return AggregateReviewStateNone
	}

	var hasChangesRequested, hasApproved, hasCommented, hasPending bool
	for _, reviewerState := range states {
		switch reviewerState.Decision {
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

// ReviewSummary is the full review picture for a PR. Its three fields have three
// different lifecycles, and reading one as though it had another's is a mistake:
//
//   - RequestedReviewers — list-time, always populated for providers that return it.
//   - ReviewerStates — fetched separately, so wrapped; may be pending, absent, or failed.
//   - AggregateState — derived locally and refined in place as better inputs arrive.
//     Not wrapped; its zero value means "not computed yet". Check IsKnown() before
//     treating it as an answer.
type ReviewSummary struct {
	// RequestedReviewers is populated from the list API for providers that return it.
	// Contains reviewer identities with a Pending decision.
	RequestedReviewers []ReviewerState

	// ReviewerStates contains submitted review decisions per reviewer.
	// Lazy-loaded via a secondary /reviews call (GitHub, Gitea) or /approvals (GitLab).
	ReviewerStates LoadResult[[]ReviewerState]

	// AggregateState is computed after ReviewerStates loads; before that it is
	// derived from RequestedReviewers alone (any requested reviewer → AggregateReviewStateRequired).
	// It stays AggregateReviewStateUnknown when neither input has produced an answer —
	// which is not the same as AggregateReviewStateNone.
	AggregateState AggregateReviewState
}
