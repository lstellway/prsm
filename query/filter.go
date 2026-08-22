package query

import (
	"fmt"
	"strings"
	"time"

	"github.com/lstellway/prsm/model"
)

// ResolvedIdentities maps provider instance name (ProviderInstance.Name) to the
// authenticated user's identity on that instance. Keying by instance rather than
// by ProviderKind avoids collapsing several instances of one kind, configured
// with different logins, into a single identity.
type ResolvedIdentities map[string]model.Author

// BaseFilterSpec holds filter fields that apply to any resource type.
type BaseFilterSpec struct {
	Author   string   // "" = no filter; "me" = authenticated user per provider instance
	Repo     []string // OR-match: "owner/repo" format
	Provider []string // OR-match: matches Provider.Name
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
// resolvedMe supplies the authenticated user's identity per provider instance,
// used to resolve "me" sentinels.
func (filterSpec PRFilterSpec) Compile(resolvedMe ResolvedIdentities) (Predicate[model.PullRequest], error) {
	predicate := Predicate[model.PullRequest](func(model.PullRequest) bool { return true })

	if filterSpec.Author != "" {
		predicate = predicate.And(matchesAuthor(filterSpec.Author, resolvedMe))
	}
	if len(filterSpec.Repo) > 0 {
		predicate = predicate.And(matchesAnyRepo(filterSpec.Repo))
	}
	if len(filterSpec.Provider) > 0 {
		predicate = predicate.And(matchesAnyProviderName(filterSpec.Provider))
	}
	if filterSpec.Reviewer != "" {
		predicate = predicate.And(matchesRequestedReviewer(filterSpec.Reviewer, resolvedMe))
	}
	if filterSpec.ReviewStatus != "" {
		aggregateState, err := parseAggregateReviewState(filterSpec.ReviewStatus)
		if err != nil {
			return nil, err
		}
		predicate = predicate.And(matchesAggregateReviewState(aggregateState))
	}
	if filterSpec.State != "" {
		state, err := parsePRState(filterSpec.State)
		if err != nil {
			return nil, err
		}
		predicate = predicate.And(matchesPRState(state))
	}
	if filterSpec.Draft != nil {
		predicate = predicate.And(matchesDraftState(*filterSpec.Draft))
	}
	if len(filterSpec.Label) > 0 {
		predicate = predicate.And(matchesAllLabels(filterSpec.Label))
	}
	if filterSpec.StalenessGTE > 0 {
		predicate = predicate.And(matchesStalenessAtLeast(filterSpec.StalenessGTE))
	}
	if filterSpec.TargetBranch != "" {
		predicate = predicate.And(matchesTargetBranchSubstring(filterSpec.TargetBranch))
	}
	if filterSpec.CIStatus != "" {
		ciState, err := parseCIState(filterSpec.CIStatus)
		if err != nil {
			return nil, err
		}
		predicate = predicate.And(matchesCIState(ciState))
	}

	return predicate, nil
}

// resolveMe returns the username to compare against for a "me" sentinel.
// The PR is matched against the identity of the provider instance it belongs to;
// the lookup is exact, since both the key and pullRequest.Provider.Name originate
// from the same configured ProviderInstance.
// Returns ("", false) when "me" cannot be resolved for that instance — which is also
// the case for an instance that is offline because its identity never resolved.
// Callers must treat an unresolved "me" as a non-match rather than comparing against "".
func resolveMe(raw string, pullRequest model.PullRequest, resolvedMe ResolvedIdentities) (string, bool) {
	if raw != "me" {
		return raw, true
	}
	if identity, ok := resolvedMe[pullRequest.Provider.Name]; ok {
		return identity.Username, true
	}
	return "", false
}

// matchesAuthor compares the PR's author against the configured username,
// resolving a "me" sentinel against the identity of the PR's provider instance.
func matchesAuthor(author string, resolvedMe ResolvedIdentities) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		target, ok := resolveMe(author, pullRequest, resolvedMe)
		if !ok {
			return false
		}
		return strings.EqualFold(pullRequest.Author.Username, target)
	}
}

// matchesRequestedReviewer matches a PR that has the given username among its
// requested reviewers, resolving a "me" sentinel against the identity of the PR's
// provider instance. Only pending review requests are considered — a reviewer who
// has already submitted a decision is matched by review status, not by this.
func matchesRequestedReviewer(reviewer string, resolvedMe ResolvedIdentities) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		target, ok := resolveMe(reviewer, pullRequest, resolvedMe)
		if !ok {
			return false
		}
		for _, reviewerState := range pullRequest.Reviews.RequestedReviewers {
			if strings.EqualFold(reviewerState.Reviewer.Username, target) {
				return true
			}
		}
		return false
	}
}

// matchesAggregateReviewState implements "unknown matches, known compares".
// AggregateState is a bare enum rather than a LoadResult, so its unknown
// is the zero value — this reads one field and never consults ReviewerStates.
//
// A derived review_required is known, so it compares: a PR already established to
// need review does not match review_status = "approved".
func matchesAggregateReviewState(status model.AggregateReviewState) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		if !pullRequest.Reviews.AggregateState.IsKnown() {
			return true
		}
		return pullRequest.Reviews.AggregateState == status
	}
}

func matchesPRState(state model.PRState) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		return pullRequest.State == state
	}
}

// matchesDraftState filters based on whether a PR is a draft.
// Checks both State == PRStateDraft (providers that encode draft as state) and
// the Draft bool (providers that expose it as a separate field alongside PRStateOpen).
func matchesDraftState(draft bool) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		isDraft := pullRequest.State == model.PRStateDraft || pullRequest.Draft
		return isDraft == draft
	}
}

// matchesAllLabels implements AND-match: the PR must carry every listed label.
func matchesAllLabels(labels []string) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		for _, required := range labels {
			found := false
			for _, label := range pullRequest.Labels {
				if strings.EqualFold(label.Name, required) {
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

// matchesAnyRepo implements OR-match: the PR must be in at least one of the listed repos.
func matchesAnyRepo(repos []string) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		fullName := pullRequest.Repo.Owner + "/" + pullRequest.Repo.Name
		for _, repo := range repos {
			if strings.EqualFold(fullName, repo) {
				return true
			}
		}
		return false
	}
}

// matchesAnyProviderName implements OR-match: the PR must be from at least one of the listed providers.
// Matches against Provider.Name (the user-assigned config alias, e.g. "github-personal"),
// consistent with FilterConfig.Provider in config/.
func matchesAnyProviderName(providers []string) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		for _, providerName := range providers {
			if strings.EqualFold(pullRequest.Provider.Name, providerName) {
				return true
			}
		}
		return false
	}
}

func matchesStalenessAtLeast(days int) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		staleness := time.Since(pullRequest.UpdatedAt)
		return int(staleness.Hours()/24) >= days
	}
}

func matchesTargetBranchSubstring(branch string) Predicate[model.PullRequest] {
	lowerBranch := strings.ToLower(branch)
	return func(pullRequest model.PullRequest) bool {
		return strings.Contains(strings.ToLower(pullRequest.TargetBranch), lowerBranch)
	}
}

// matchesCIState lets pending items pass through, remaining visible until
// their CI data loads. LoadStateAbsent and LoadStateError are treated as
// CIStateNone for filter evaluation.
func matchesCIState(status model.CIState) Predicate[model.PullRequest] {
	return func(pullRequest model.PullRequest) bool {
		if pullRequest.CI.IsPending() {
			return true
		}
		actualState := model.CIStateNone
		if ciStatus, ok := pullRequest.CI.Get(); ok {
			actualState = ciStatus.State
		}
		return actualState == status
	}
}

func parseAggregateReviewState(status string) (model.AggregateReviewState, error) {
	switch status {
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
		// The sentinel is deliberate: on the error path there is no state to report.
		// Callers must check err — the value is meaningless, not a default.
		// "unknown" is not accepted as user input; it is prsm's own bookkeeping.
		return model.AggregateReviewStateUnknown, fmt.Errorf("invalid review_status %q: must be one of approved, changes_requested, review_required, commented, none", status)
	}
}

func parsePRState(state string) (model.PRState, error) {
	switch state {
	case "open":
		return model.PRStateOpen, nil
	case "closed":
		return model.PRStateClosed, nil
	case "merged":
		return model.PRStateMerged, nil
	case "draft":
		return model.PRStateDraft, nil
	default:
		return model.PRStateUnknown, fmt.Errorf("invalid state %q: must be one of open, closed, merged, draft", state)
	}
}

func parseCIState(status string) (model.CIState, error) {
	switch status {
	case "passing":
		return model.CIStatePassing, nil
	case "failing":
		return model.CIStateFailing, nil
	case "pending":
		return model.CIStatePending, nil
	case "none":
		return model.CIStateNone, nil
	default:
		return model.CIStateUnknown, fmt.Errorf("invalid ci_status %q: must be one of passing, failing, pending, none", status)
	}
}
