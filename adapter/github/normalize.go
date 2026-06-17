package github

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/model"
)

// normalizePR maps a GitHub REST pull request to model.PullRequest.
// All list-time fields are populated; lazy fields (CI, ReviewerStates, Diff)
// are set to LoadStatePending.
//
// Fields not available from the REST list endpoint:
//   - CommentCount: not returned by GET /pulls; set to zero until a future lazy load
//   - Mergeable: not computed by GitHub at list time; set to MergeableStateUnknown
func normalizePR(pr *gogithub.PullRequest, owner, repo string, instance model.ProviderInstance) model.PullRequest {
	result := model.PullRequest{
		ProviderID:   pr.GetNodeID(),
		Number:       pr.GetNumber(),
		Provider:     instance,
		URL:          pr.GetHTMLURL(),
		Title:        pr.GetTitle(),
		Body:         pr.GetBody(),
		SourceBranch: pr.GetHead().GetRef(),
		TargetBranch: pr.GetBase().GetRef(),
		HeadSHA:      pr.GetHead().GetSHA(),
		Draft:        pr.GetDraft(),
		Repo:         model.Repository{Owner: owner, Name: repo},
		CI:           model.Pending[model.CIStatus](),
		Diff:         model.Pending[model.DiffStats](),
		CreatedAt:    pr.GetCreatedAt().Time,
		UpdatedAt:    pr.GetUpdatedAt().Time,
	}

	if mergedAt := pr.GetMergedAt(); !mergedAt.IsZero() {
		cp := mergedAt.Time
		result.MergedAt = &cp
	}

	result.State = normalizePRState(pr.GetState(), pr.GetDraft())
	result.Author = normalizeIdentity(pr.GetUser().GetLogin(), "", pr.GetUser().GetAvatarURL())
	result.Labels = normalizeLabels(pr.Labels)
	result.Reviews = normalizeReviewSummary(pr.RequestedReviewers)

	return result
}

func normalizePRState(state string, isDraft bool) model.PRState {
	if isDraft {
		return model.PRStateDraft
	}
	switch strings.ToUpper(state) {
	case "OPEN":
		return model.PRStateOpen
	case "CLOSED":
		return model.PRStateClosed
	case "MERGED":
		return model.PRStateMerged
	default:
		return model.PRStateOpen
	}
}

func normalizeIdentity(login, name, avatarURL string) model.Identity {
	if name == "" {
		name = login
	}
	return model.Identity{
		Username:    login,
		DisplayName: name,
		AvatarURL:   avatarURL,
	}
}

func normalizeLabels(labels []*gogithub.Label) []model.Label {
	if len(labels) == 0 {
		return nil
	}
	result := make([]model.Label, len(labels))
	for i, l := range labels {
		color := l.GetColor()
		if color != "" && !strings.HasPrefix(color, "#") {
			color = "#" + color
		}
		result[i] = model.Label{Name: l.GetName(), Color: color}
	}
	return result
}

// normalizeReviewSummary builds a ReviewSummary from the REST list-time
// requested_reviewers field. ReviewerStates starts as Pending.
// AggregateState is conservatively set to ReviewRequired if any requestee exists.
func normalizeReviewSummary(reviewers []*gogithub.User) model.ReviewSummary {
	rs := model.ReviewSummary{
		ReviewerStates: model.Pending[[]model.ReviewerState](),
	}

	for _, u := range reviewers {
		if u.GetLogin() == "" {
			continue
		}
		rs.RequestedReviewers = append(rs.RequestedReviewers, model.ReviewerState{
			Reviewer: model.Identity{
				Username:    u.GetLogin(),
				DisplayName: u.GetLogin(), // display name not available in REST list user objects
				AvatarURL:   u.GetAvatarURL(),
			},
			Decision: model.ReviewDecisionPending,
		})
	}

	if len(rs.RequestedReviewers) > 0 {
		rs.AggregateState = model.AggregateReviewStateRequired
	}

	return rs
}

// normalizeAggregateState computes AggregateReviewState from loaded reviewer decisions.
func normalizeAggregateState(states []model.ReviewerState) model.AggregateReviewState {
	if len(states) == 0 {
		return model.AggregateReviewStateNone
	}

	var hasChangesRequested, hasApproved, hasCommented, hasPending bool
	for _, s := range states {
		switch s.Decision {
		case model.ReviewDecisionChangesRequested:
			hasChangesRequested = true
		case model.ReviewDecisionApproved:
			hasApproved = true
		case model.ReviewDecisionCommented:
			hasCommented = true
		case model.ReviewDecisionPending:
			hasPending = true
		}
	}

	switch {
	case hasChangesRequested:
		return model.AggregateReviewStateChangesRequested
	case hasPending:
		return model.AggregateReviewStateRequired
	case hasApproved:
		return model.AggregateReviewStateApproved
	case hasCommented:
		return model.AggregateReviewStateCommented
	default:
		return model.AggregateReviewStateNone
	}
}

// normalizeCIStatus maps GitHub check run conclusions to CIStatus.
func normalizeCIStatus(runs []*gogithub.CheckRun) model.CIStatus {
	if len(runs) == 0 {
		return model.CIStatus{State: model.CIStateNone}
	}

	var passing, failing, pending int
	for _, r := range runs {
		conclusion := strings.ToLower(r.GetConclusion())
		status := strings.ToLower(r.GetStatus())

		switch {
		case conclusion == "success":
			passing++
		case conclusion == "failure" || conclusion == "timed_out" || conclusion == "action_required":
			failing++
		case status == "in_progress" || status == "queued" || conclusion == "":
			pending++
		default:
			// "skipped", "neutral", "cancelled", "stale" — counted as passing.
			// DESIGN DECISION NEEDED: these are not failures but also not successes;
			// revisit when the TUI needs to distinguish them for display.
			passing++
		}
	}

	var state model.CIState
	switch {
	case failing > 0:
		state = model.CIStateFailing
	case pending > 0:
		state = model.CIStatePending
	default:
		state = model.CIStatePassing
	}

	total := passing + failing + pending
	summary := fmt.Sprintf("%d/%d checks passed", passing, total)
	if failing > 0 {
		summary = fmt.Sprintf("%d/%d checks passed, %d failed", passing, total, failing)
	}

	return model.CIStatus{State: state, Summary: summary}
}

// normalizeReviewDecision maps a GitHub review state string to ReviewDecision.
func normalizeReviewDecision(state string) model.ReviewDecision {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return model.ReviewDecisionApproved
	case "CHANGES_REQUESTED":
		return model.ReviewDecisionChangesRequested
	case "COMMENTED":
		return model.ReviewDecisionCommented
	case "DISMISSED":
		return model.ReviewDecisionDismissed
	default:
		return model.ReviewDecisionPending
	}
}

// normalizeReviewerStates maps GitHub REST review objects to []ReviewerState.
// Only the most-recent review per reviewer is retained, matching GitHub's own
// PR review UI which collapses multiple reviews per reviewer to their latest state.
func normalizeReviewerStates(reviews []*gogithub.PullRequestReview) []model.ReviewerState {
	seen := make(map[string]model.ReviewDecision)
	names := make(map[string]string)
	for _, r := range reviews {
		login := r.GetUser().GetLogin()
		if login == "" {
			continue
		}
		seen[login] = normalizeReviewDecision(r.GetState())
		names[login] = r.GetUser().GetName()
	}

	if len(seen) == 0 {
		return nil
	}

	states := make([]model.ReviewerState, 0, len(seen))
	for login, decision := range seen {
		name := names[login]
		if name == "" {
			name = login
		}
		states = append(states, model.ReviewerState{
			Reviewer: model.Identity{
				Username:    login,
				DisplayName: name,
			},
			Decision: decision,
		})
	}
	slices.SortFunc(states, func(a, b model.ReviewerState) int {
		return cmp.Compare(a.Reviewer.Username, b.Reviewer.Username)
	})
	return states
}
