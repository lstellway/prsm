package github

import (
	"fmt"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/model"
)

// normalizePR maps a GitHub GraphQL pull request node to model.PullRequest.
// All list-time fields are populated; lazy fields (CI, ReviewerStates, Diff)
// are set to LoadStatePending.
func normalizePR(node prNode, instance model.ProviderInstance) model.PullRequest {
	pr := model.PullRequest{
		ProviderID:   node.ID,
		Number:       node.Number,
		Provider:     instance,
		URL:          node.URL,
		Title:        node.Title,
		Body:         node.Body,
		SourceBranch: node.HeadRefName,
		TargetBranch: node.BaseRefName,
		HeadSHA:      node.HeadRefOid,
		Draft:        node.IsDraft,
		CommentCount: node.Comments.TotalCount,
		Repo: model.Repository{
			Owner: node.Repository.Owner.Login,
			Name:  node.Repository.Name,
		},
		CI:        model.Pending[model.CIStatus](),
		Diff:      model.Pending[model.DiffStats](),
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}

	if !node.MergedAt.IsZero() {
		t := node.MergedAt
		pr.MergedAt = &t
	}

	pr.State = normalizePRState(node.State, node.IsDraft)
	pr.Mergeable = normalizeMergeable(node.Mergeable)
	pr.Author = normalizeIdentity(node.Author.Login, node.Author.Name, node.Author.AvatarURL)
	pr.Labels = normalizeLabels(node.Labels.Nodes)
	pr.Reviews = normalizeReviewSummary(node.ReviewRequests.Nodes)

	return pr
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

func normalizeMergeable(s string) model.MergeableState {
	switch strings.ToUpper(s) {
	case "MERGEABLE":
		return model.MergeableStateMergeable
	case "CONFLICTING":
		return model.MergeableStateConflicting
	default:
		return model.MergeableStateUnknown
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

func normalizeLabels(nodes []prLabel) []model.Label {
	if len(nodes) == 0 {
		return nil
	}
	labels := make([]model.Label, len(nodes))
	for i, l := range nodes {
		color := l.Color
		if color != "" && !strings.HasPrefix(color, "#") {
			color = "#" + color
		}
		labels[i] = model.Label{Name: l.Name, Color: color}
	}
	return labels
}

// normalizeReviewSummary builds a ReviewSummary from list-time GraphQL data.
// RequestedReviewers is populated; ReviewerStates starts as Pending.
// AggregateState is conservatively derived: ReviewRequired if any requestee exists.
func normalizeReviewSummary(nodes []reviewRequestNode) model.ReviewSummary {
	rs := model.ReviewSummary{
		ReviewerStates: model.Pending[[]model.ReviewerState](),
	}

	for _, n := range nodes {
		if n.RequestedReviewer.Login == "" {
			continue
		}
		rs.RequestedReviewers = append(rs.RequestedReviewers, model.ReviewerState{
			Reviewer: model.Identity{
				Username:    n.RequestedReviewer.Login,
				DisplayName: n.RequestedReviewer.Name,
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
// Only the most-recent review per reviewer is retained.
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
	return states
}

// GraphQL response type definitions.

type prNode struct {
	ID             string
	Number         int
	Title          string
	Body           string
	URL            string
	State          string
	IsDraft        bool
	HeadRefName    string
	BaseRefName    string
	HeadRefOid     string
	Mergeable      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	MergedAt       time.Time
	Author         prAuthor
	Labels         struct{ Nodes []prLabel }
	ReviewRequests struct{ Nodes []reviewRequestNode }
	Comments       struct{ TotalCount int }
	Repository     struct {
		Name  string
		Owner struct{ Login string }
	}
}

type prAuthor struct {
	Login     string
	Name      string
	AvatarURL string
}

type prLabel struct {
	Name  string
	Color string
}

type reviewRequestNode struct {
	RequestedReviewer struct {
		Login string
		Name  string
	}
}
