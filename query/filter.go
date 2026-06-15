package query

import (
	"fmt"
	"strings"
	"time"

	"github.com/lstellway/prsm/model"
)

// BaseFilterSpec holds filter fields that apply to any resource type.
type BaseFilterSpec struct {
	Author   string   // "" = no filter; "me" = authenticated user per provider
	Repo     []string // OR-match: "owner/repo" format
	Provider []string // OR-match: matches Provider.Account
}

// PRFilterSpec is the serializable filter specification for a named view over pull requests.
// Embeds BaseFilterSpec and adds PR-specific fields.
// Compile converts it to a Predicate[model.PullRequest] for runtime evaluation.
type PRFilterSpec struct {
	BaseFilterSpec
	Reviewer     string   // "" = no filter; "me" = authenticated user
	ReviewStatus string   // "approved" | "changes_requested" | "review_required" | "commented" | "none"
	State        string   // "open" | "closed" | "merged" | "draft"
	Draft        *bool    // nil = no filter; &true = drafts only; &false = no drafts
	Label        []string // AND-match: PR must carry all listed labels
	StalenessGTE int      // >= N days since UpdatedAt; 0 = no filter
	TargetBranch string   // substring match; "" = no filter
	CIStatus     string   // "passing" | "failing" | "pending" | "none"; pending items pass through
}

// Compile builds a Predicate[model.PullRequest] from the spec.
// resolvedMe maps provider kind to the authenticated user's identity, used to resolve "me" sentinels.
func (s PRFilterSpec) Compile(resolvedMe map[model.ProviderKind]model.Author) (Predicate[model.PullRequest], error) {
	pred := Predicate[model.PullRequest](func(model.PullRequest) bool { return true })

	if s.Author != "" {
		pred = pred.And(authorPred(s.Author, resolvedMe))
	}
	if len(s.Repo) > 0 {
		pred = pred.And(repoPred(s.Repo))
	}
	if len(s.Provider) > 0 {
		pred = pred.And(providerPred(s.Provider))
	}
	if s.Reviewer != "" {
		pred = pred.And(reviewerPred(s.Reviewer, resolvedMe))
	}
	if s.ReviewStatus != "" {
		aggState, err := parseAggregateReviewState(s.ReviewStatus)
		if err != nil {
			return nil, err
		}
		pred = pred.And(reviewStatusPred(aggState))
	}
	if s.State != "" {
		state, err := parsePRState(s.State)
		if err != nil {
			return nil, err
		}
		pred = pred.And(statePred(state))
	}
	if s.Draft != nil {
		pred = pred.And(draftPred(*s.Draft))
	}
	if len(s.Label) > 0 {
		pred = pred.And(labelPred(s.Label))
	}
	if s.StalenessGTE > 0 {
		pred = pred.And(stalenessPred(s.StalenessGTE))
	}
	if s.TargetBranch != "" {
		pred = pred.And(targetBranchPred(s.TargetBranch))
	}
	if s.CIStatus != "" {
		ciState, err := parseCIState(s.CIStatus)
		if err != nil {
			return nil, err
		}
		pred = pred.And(ciStatusPred(ciState))
	}

	return pred, nil
}

// resolveMe returns the username to compare against for a "me" sentinel.
// Returns ("", false) when "me" cannot be resolved for the PR's provider kind —
// callers must treat an unresolved "me" as a non-match rather than comparing against "".
func resolveMe(raw string, pr model.PullRequest, resolvedMe map[model.ProviderKind]model.Author) (string, bool) {
	if raw != "me" {
		return raw, true
	}
	if identity, ok := resolvedMe[pr.Provider.Kind]; ok {
		return identity.Username, true
	}
	return "", false
}

func authorPred(author string, resolvedMe map[model.ProviderKind]model.Author) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		target, ok := resolveMe(author, pr, resolvedMe)
		if !ok {
			return false
		}
		return strings.EqualFold(pr.Author.Username, target)
	}
}

func reviewerPred(reviewer string, resolvedMe map[model.ProviderKind]model.Author) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		target, ok := resolveMe(reviewer, pr, resolvedMe)
		if !ok {
			return false
		}
		for _, rs := range pr.Reviews.RequestedReviewers {
			if strings.EqualFold(rs.Reviewer.Username, target) {
				return true
			}
		}
		return false
	}
}

func reviewStatusPred(status model.AggregateReviewState) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		return pr.Reviews.AggregateState == status
	}
}

func statePred(state model.PRState) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		return pr.State == state
	}
}

// draftPred filters based on whether a PR is a draft.
// Checks both State == PRStateDraft (providers that encode draft as state) and
// the Draft bool (providers that expose it as a separate field alongside PRStateOpen).
func draftPred(draft bool) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		isDraft := pr.State == model.PRStateDraft || pr.Draft
		return isDraft == draft
	}
}

// labelPred implements AND-match: the PR must carry every listed label.
func labelPred(labels []string) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		for _, required := range labels {
			found := false
			for _, l := range pr.Labels {
				if strings.EqualFold(l.Name, required) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
}

// repoPred implements OR-match: the PR must be in at least one of the listed repos.
func repoPred(repos []string) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		full := pr.Repo.Owner + "/" + pr.Repo.Name
		for _, r := range repos {
			if strings.EqualFold(full, r) {
				return true
			}
		}
		return false
	}
}

// providerPred implements OR-match: the PR must be from at least one of the listed providers.
// Matches against Provider.Name (the user-assigned config alias, e.g. "github-personal"),
// consistent with FilterConfig.Provider in config/.
func providerPred(providers []string) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		for _, p := range providers {
			if strings.EqualFold(pr.Provider.Name, p) {
				return true
			}
		}
		return false
	}
}

func stalenessPred(days int) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		staleness := time.Since(pr.UpdatedAt)
		return int(staleness.Hours()/24) >= days
	}
}

func targetBranchPred(branch string) Predicate[model.PullRequest] {
	lower := strings.ToLower(branch)
	return func(pr model.PullRequest) bool {
		return strings.Contains(strings.ToLower(pr.TargetBranch), lower)
	}
}

// ciStatusPred implements Option C from ADR-006: pending items pass through,
// remaining visible until their CI data loads. LoadStateAbsent and LoadStateError
// are treated as CIStateNone for filter evaluation.
func ciStatusPred(status model.CIState) Predicate[model.PullRequest] {
	return func(pr model.PullRequest) bool {
		if pr.CI.IsPending() {
			return true
		}
		actual := model.CIStateNone
		if ci, ok := pr.CI.Get(); ok {
			actual = ci.State
		}
		return actual == status
	}
}

func parseAggregateReviewState(s string) (model.AggregateReviewState, error) {
	switch s {
	case "none":
		return model.AggregateReviewStateNone, nil
	case "review_required":
		return model.AggregateReviewStateRequired, nil
	case "approved":
		return model.AggregateReviewStateApproved, nil
	case "changes_requested":
		return model.AggregateReviewStateChangesRequested, nil
	case "commented":
		return model.AggregateReviewStateCommented, nil
	default:
		return "", fmt.Errorf("invalid review_status %q: must be one of approved, changes_requested, review_required, commented, none", s)
	}
}

func parsePRState(s string) (model.PRState, error) {
	switch s {
	case "open":
		return model.PRStateOpen, nil
	case "closed":
		return model.PRStateClosed, nil
	case "merged":
		return model.PRStateMerged, nil
	case "draft":
		return model.PRStateDraft, nil
	default:
		return "", fmt.Errorf("invalid state %q: must be one of open, closed, merged, draft", s)
	}
}

func parseCIState(s string) (model.CIState, error) {
	switch s {
	case "passing":
		return model.CIStatePassing, nil
	case "failing":
		return model.CIStateFailing, nil
	case "pending":
		return model.CIStatePending, nil
	case "none":
		return model.CIStateNone, nil
	default:
		return "", fmt.Errorf("invalid ci_status %q: must be one of passing, failing, pending, none", s)
	}
}
