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
func normalizePR(githubPullRequest *gogithub.PullRequest, owner, repo string, instance model.ProviderInstance) model.PullRequest {
	result := model.PullRequest{
		ProviderID:   githubPullRequest.GetNodeID(),
		Number:       githubPullRequest.GetNumber(),
		Provider:     instance,
		URL:          githubPullRequest.GetHTMLURL(),
		Title:        githubPullRequest.GetTitle(),
		Body:         githubPullRequest.GetBody(),
		SourceBranch: githubPullRequest.GetHead().GetRef(),
		TargetBranch: githubPullRequest.GetBase().GetRef(),
		HeadSHA:      githubPullRequest.GetHead().GetSHA(),
		Draft:        githubPullRequest.GetDraft(),
		Repo:         model.Repository{Owner: owner, Name: repo},
		CI:           model.Pending[model.CIStatus](),
		Diff:         model.Pending[model.DiffStats](),
		CreatedAt:    githubPullRequest.GetCreatedAt().Time,
		UpdatedAt:    githubPullRequest.GetUpdatedAt().Time,
	}

	if mergedAt := githubPullRequest.GetMergedAt(); !mergedAt.IsZero() {
		mergedAtCopy := mergedAt.Time
		result.MergedAt = &mergedAtCopy
	}

	result.State = normalizePRState(githubPullRequest.GetState(), githubPullRequest.GetDraft(), result.MergedAt != nil)
	result.Author = normalizeIdentity(githubPullRequest.GetUser().GetLogin(), "", githubPullRequest.GetUser().GetAvatarURL())
	result.Labels = normalizeLabels(githubPullRequest.Labels)
	result.Reviews = normalizeReviewSummary(githubPullRequest.RequestedReviewers)

	return result
}

func normalizePRState(state string, isDraft bool, isMerged bool) model.PRState {
	// isMerged is checked first: merged_at is the authoritative signal since
	// GitHub's list endpoint returns state="closed" for merged PRs.
	if isMerged {
		return model.PRStateMerged
	}
	if isDraft {
		return model.PRStateDraft
	}
	switch strings.ToUpper(state) {
	case "OPEN":
		return model.PRStateOpen
	case "CLOSED":
		return model.PRStateClosed
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
	for index, label := range labels {
		color := label.GetColor()
		if color != "" && !strings.HasPrefix(color, "#") {
			color = "#" + color
		}
		result[index] = model.Label{Name: label.GetName(), Color: color}
	}
	return result
}

// normalizeReviewSummary builds a ReviewSummary from the REST list-time
// requested_reviewers field. ReviewerStates starts as Pending.
// AggregateState is conservatively set to ReviewRequired if any requestee exists.
func normalizeReviewSummary(reviewers []*gogithub.User) model.ReviewSummary {
	reviewSummary := model.ReviewSummary{
		ReviewerStates: model.Pending[[]model.ReviewerState](),
	}

	for _, user := range reviewers {
		if user.GetLogin() == "" {
			continue
		}
		reviewSummary.RequestedReviewers = append(reviewSummary.RequestedReviewers, model.ReviewerState{
			Reviewer: model.Identity{
				Username:    user.GetLogin(),
				DisplayName: user.GetLogin(), // display name not available in REST list user objects
				AvatarURL:   user.GetAvatarURL(),
			},
			Decision: model.ReviewDecisionPending,
		})
	}

	if len(reviewSummary.RequestedReviewers) > 0 {
		reviewSummary.AggregateState = model.AggregateReviewStateRequired
	}

	return reviewSummary
}

// normalizeCIStatus maps GitHub check run conclusions to CIStatus.
func normalizeCIStatus(runs []*gogithub.CheckRun) model.CIStatus {
	if len(runs) == 0 {
		return model.CIStatus{State: model.CIStateNone}
	}

	var passing, failing, pending int
	for _, checkRun := range runs {
		conclusion := strings.ToLower(checkRun.GetConclusion())
		status := strings.ToLower(checkRun.GetStatus())

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
	for _, review := range reviews {
		login := review.GetUser().GetLogin()
		if login == "" {
			continue
		}
		seen[login] = normalizeReviewDecision(review.GetState())
		names[login] = review.GetUser().GetName()
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
	slices.SortFunc(states, func(left, right model.ReviewerState) int {
		return cmp.Compare(left.Reviewer.Username, right.Reviewer.Username)
	})
	return states
}
