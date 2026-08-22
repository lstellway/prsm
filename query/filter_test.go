package query_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func boolPointer(value bool) *bool { return &value }

// resolvedMe is keyed by provider instance name, matching the Provider.Name that
// githubPR builds. Nothing is resolved for any other instance, so "me" against a
// PR from one of those must not match.
var resolvedMe = query.ResolvedIdentities{
	"github-personal": {Username: "bob"},
}

func githubPR(options ...func(*model.PullRequest)) model.PullRequest {
	pullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub, Account: "acme"},
		State:    model.PRStateOpen,
	}
	for _, option := range options {
		option(&pullRequest)
	}
	return pullRequest
}

func withReviewer(username string) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) {
		pullRequest.Reviews.RequestedReviewers = append(pullRequest.Reviews.RequestedReviewers,
			model.ReviewerState{Reviewer: model.Reviewer{Username: username}},
		)
	}
}

func withAuthor(username string) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) { pullRequest.Author.Username = username }
}

func withState(state model.PRState) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) { pullRequest.State = state }
}

func withLabels(names ...string) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) {
		for _, name := range names {
			pullRequest.Labels = append(pullRequest.Labels, model.Label{Name: name})
		}
	}
}

func withRepo(owner, name string) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) {
		pullRequest.Repo = model.Repository{Owner: owner, Name: name}
	}
}

func withAggregateReview(state model.AggregateReviewState) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) { pullRequest.Reviews.AggregateState = state }
}

func withTargetBranch(branch string) func(*model.PullRequest) {
	return func(pullRequest *model.PullRequest) { pullRequest.TargetBranch = branch }
}

// TestCompile_ReviewerMeAndNoDraft is the primary acceptance criterion:
// PRFilterSpec{Reviewer: "me", Draft: boolPointer(false)}.Compile(resolvedMe) must correctly
// filter a mixed list.
func TestCompile_ReviewerMeAndNoDraft(t *testing.T) {
	filterSpec := query.PRFilterSpec{
		Reviewer: "me",
		Draft:    boolPointer(false),
	}
	predicate, err := filterSpec.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	tests := []struct {
		name        string
		pullRequest model.PullRequest
		want        bool
	}{
		{
			name:        "open PR with bob as reviewer → pass",
			pullRequest: githubPR(withReviewer("bob")),
			want:        true,
		},
		{
			name:        "draft PR with bob as reviewer → fail (is draft)",
			pullRequest: githubPR(withReviewer("bob"), withState(model.PRStateDraft)),
			want:        false,
		},
		{
			name:        "open PR without bob as reviewer → fail",
			pullRequest: githubPR(withReviewer("alice")),
			want:        false,
		},
		{
			name:        "open PR with no reviewers → fail",
			pullRequest: githubPR(),
			want:        false,
		},
		{
			name:        "open PR with bob and alice as reviewers → pass",
			pullRequest: githubPR(withReviewer("alice"), withReviewer("bob")),
			want:        true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := predicate(testCase.pullRequest); got != testCase.want {
				t.Errorf("predicate() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCompile_Author(t *testing.T) {
	predicate, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withAuthor("bob"))) {
		t.Error("expected PR authored by bob to match author=me")
	}
	if predicate(githubPR(withAuthor("alice"))) {
		t.Error("expected PR authored by alice to not match author=me")
	}
}

func TestCompile_LabelANDMatch(t *testing.T) {
	predicate, err := query.PRFilterSpec{Label: []string{"bug", "priority"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withLabels("bug", "priority"))) {
		t.Error("expected PR with both labels to match")
	}
	if predicate(githubPR(withLabels("bug"))) {
		t.Error("expected PR with only one label to not match")
	}
	if predicate(githubPR()) {
		t.Error("expected PR with no labels to not match")
	}
}

func TestCompile_RepoORMatch(t *testing.T) {
	predicate, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Repo: []string{"acme/api", "acme/frontend"}}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withRepo("acme", "api"))) {
		t.Error("expected acme/api to match")
	}
	if !predicate(githubPR(withRepo("acme", "frontend"))) {
		t.Error("expected acme/frontend to match")
	}
	if predicate(githubPR(withRepo("acme", "backend"))) {
		t.Error("expected acme/backend to not match")
	}
}

func TestCompile_State(t *testing.T) {
	predicate, err := query.PRFilterSpec{State: "open"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withState(model.PRStateOpen))) {
		t.Error("expected open PR to match")
	}
	if predicate(githubPR(withState(model.PRStateMerged))) {
		t.Error("expected merged PR to not match")
	}
}

func TestCompile_ReviewStatus(t *testing.T) {
	predicate, err := query.PRFilterSpec{ReviewStatus: "review_required"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withAggregateReview(model.AggregateReviewStateRequired))) {
		t.Error("expected review_required PR to match")
	}
	if predicate(githubPR(withAggregateReview(model.AggregateReviewStateApproved))) {
		t.Error("expected approved PR to not match")
	}
}

func TestCompile_ReviewStatus_None(t *testing.T) {
	predicate, err := query.PRFilterSpec{ReviewStatus: "none"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withAggregateReview(model.AggregateReviewStateNone))) {
		t.Error("expected a PR computed to have no reviews to match review_status=none")
	}
	if predicate(githubPR(withAggregateReview(model.AggregateReviewStateApproved))) {
		t.Error("expected approved PR to not match review_status=none")
	}
}

// TestCompile_ReviewStatus_UnknownMatchesEverything pins "unknown matches, known
// compares": an uncomputed aggregate stays visible and drains out of the view as
// its data arrives. Excluding it instead would make approved, changes_requested,
// commented and none match nothing at all in the shipping GitHub adapter, which
// only ever derives review_required or nothing.
func TestCompile_ReviewStatus_UnknownMatchesEverything(t *testing.T) {
	for _, status := range []string{"none", "approved", "changes_requested", "review_required", "commented"} {
		t.Run(status, func(t *testing.T) {
			predicate, err := query.PRFilterSpec{ReviewStatus: status}.Compile(resolvedMe)
			if err != nil {
				t.Fatalf("Compile() error: %v", err)
			}
			if !predicate(githubPR()) {
				t.Errorf("PR with an uncomputed aggregate did not match review_status=%s", status)
			}
		})
	}
}

// TestCompile_ReviewStatus_DerivedRequiredIsCompared is the other half of §2(d), and
// the reason the rule reads AggregateState rather than ReviewerStates.IsPending().
// A PR with requested reviewers has a derived review_required — a known value, even
// though the full reviewer states have not loaded — so it compares. A blanket
// "still loading → match" would discard that and let a PR known to need review
// match review_status=approved.
func TestCompile_ReviewStatus_DerivedRequiredIsCompared(t *testing.T) {
	derivedRequired := githubPR(withAggregateReview(model.AggregateReviewStateRequired))

	approvedPredicate, err := query.PRFilterSpec{ReviewStatus: "approved"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if approvedPredicate(derivedRequired) {
		t.Error("a PR derived as review_required matched review_status=approved")
	}

	requiredPredicate, err := query.PRFilterSpec{ReviewStatus: "review_required"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if !requiredPredicate(derivedRequired) {
		t.Error("a PR derived as review_required did not match review_status=review_required")
	}
}

// TestCompile_ReviewStatus_UnknownIsNotUserInput keeps prsm's internal bookkeeping
// out of the filter vocabulary: "unknown" is a state prsm assigns, not one a user asks for.
func TestCompile_ReviewStatus_UnknownIsNotUserInput(t *testing.T) {
	if _, err := (query.PRFilterSpec{ReviewStatus: "unknown"}).Compile(resolvedMe); err == nil {
		t.Error("expected review_status=unknown to be rejected")
	}
}

func TestCompile_TargetBranch(t *testing.T) {
	predicate, err := query.PRFilterSpec{TargetBranch: "main"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !predicate(githubPR(withTargetBranch("main"))) {
		t.Error("expected PR targeting main to match")
	}
	if !predicate(githubPR(withTargetBranch("release/main-2026"))) {
		t.Error("expected PR targeting release/main-2026 to match (substring)")
	}
	if predicate(githubPR(withTargetBranch("develop"))) {
		t.Error("expected PR targeting develop to not match")
	}
}

func TestCompile_CIStatus_PendingPassthrough(t *testing.T) {
	predicate, err := query.PRFilterSpec{CIStatus: "failing"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	pendingCIPullRequest := githubPR()
	// CI is LoadStatePending (zero value) — must pass through
	if !predicate(pendingCIPullRequest) {
		t.Error("expected pending CI PR to pass through ci_status filter (Option C)")
	}

	failingCIPullRequest := githubPR()
	failingCIPullRequest.CI = model.Loaded(model.CIStatus{State: model.CIStateFailing})
	if !predicate(failingCIPullRequest) {
		t.Error("expected failing CI PR to match ci_status=failing")
	}

	passingCIPullRequest := githubPR()
	passingCIPullRequest.CI = model.Loaded(model.CIStatus{State: model.CIStatePassing})
	if predicate(passingCIPullRequest) {
		t.Error("expected passing CI PR to not match ci_status=failing")
	}
}

func TestCompile_InvalidReviewStatus(t *testing.T) {
	_, err := query.PRFilterSpec{ReviewStatus: "bogus"}.Compile(resolvedMe)
	if err == nil {
		t.Error("expected error for invalid review_status")
	}
}

func TestCompile_InvalidState(t *testing.T) {
	_, err := query.PRFilterSpec{State: "bogus"}.Compile(resolvedMe)
	if err == nil {
		t.Error("expected error for invalid state")
	}
}

func TestCompile_InvalidCIStatus(t *testing.T) {
	_, err := query.PRFilterSpec{CIStatus: "bogus"}.Compile(resolvedMe)
	if err == nil {
		t.Error("expected error for invalid ci_status")
	}
}

func TestCompile_ProviderORMatch(t *testing.T) {
	predicate, err := query.PRFilterSpec{
		BaseFilterSpec: query.BaseFilterSpec{Provider: []string{"github-personal", "gitlab-work"}},
	}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	githubPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub},
		State:    model.PRStateOpen,
	}
	gitlabPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "gitlab-work", Kind: model.ProviderGitLab},
		State:    model.PRStateOpen,
	}
	otherPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "codeberg", Kind: model.ProviderGitea},
		State:    model.PRStateOpen,
	}

	if !predicate(githubPullRequest) {
		t.Error("expected github-personal to match")
	}
	if !predicate(gitlabPullRequest) {
		t.Error("expected gitlab-work to match")
	}
	if predicate(otherPullRequest) {
		t.Error("expected codeberg to not match")
	}
}

func TestCompile_DraftBoolField(t *testing.T) {
	predicate, err := query.PRFilterSpec{Draft: boolPointer(false)}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// Provider sets pullRequest.Draft = true with State = open (e.g. GitHub before it adopted PRStateDraft)
	draftBoolPullRequest := githubPR()
	draftBoolPullRequest.State = model.PRStateOpen
	draftBoolPullRequest.Draft = true
	if predicate(draftBoolPullRequest) {
		t.Error("expected PR with Draft=true to be excluded by draft=false filter")
	}

	// Draft=false, State=open → passes
	if !predicate(githubPR()) {
		t.Error("expected open non-draft PR to pass draft=false filter")
	}
}

func TestCompile_ResolveMe_UnknownProvider(t *testing.T) {
	// resolvedMe has no entry for the "gitlab-work" instance — "me" must not match its
	// PRs even though the author's username collides with the resolved GitHub identity.
	gitlabPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "gitlab-work", Kind: model.ProviderGitLab},
		Author:   model.Author{Username: "bob"},
		State:    model.PRStateOpen,
	}

	predicate, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if predicate(gitlabPullRequest) {
		t.Error("expected 'me' with unresolved provider to match nothing, not any GitLab PR")
	}
}

func TestCompile_ResolveMe_EmptyUsername(t *testing.T) {
	// A PR with an empty Author.Username must not match author="me" when "me" is unresolved
	emptyAuthorPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "gitlab-work", Kind: model.ProviderGitLab},
		Author:   model.Author{Username: ""},
		State:    model.PRStateOpen,
	}

	predicate, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if predicate(emptyAuthorPullRequest) {
		t.Error("unresolved 'me' must not match a PR with an empty author username")
	}
}

// TestCompile_ResolveMe_TwoInstancesOfOneKind is the regression test for keying
// identities by provider instance rather than by ProviderKind. github.com and a
// GitHub Enterprise Server are both ProviderGitHub but authenticate as different
// logins; keying by kind collapsed them so one instance's identity won and "me"
// silently resolved wrong for the other.
func TestCompile_ResolveMe_TwoInstancesOfOneKind(t *testing.T) {
	identities := query.ResolvedIdentities{
		"github-personal": {Username: "bob"},
		"github-work":     {Username: "robert"},
	}

	pullRequestOn := func(instanceName, username string) model.PullRequest {
		return model.PullRequest{
			Provider: model.ProviderInstance{Name: instanceName, Kind: model.ProviderGitHub},
			Author:   model.Author{Username: username},
			Reviews:  model.ReviewSummary{RequestedReviewers: []model.ReviewerState{{Reviewer: model.Reviewer{Username: username}}}},
			State:    model.PRStateOpen,
		}
	}

	tests := []struct {
		name        string
		pullRequest model.PullRequest
		want        bool
	}{
		{
			name:        "personal instance, personal login → match",
			pullRequest: pullRequestOn("github-personal", "bob"),
			want:        true,
		},
		{
			name:        "work instance, work login → match",
			pullRequest: pullRequestOn("github-work", "robert"),
			want:        true,
		},
		{
			name:        "personal instance, work login → no match",
			pullRequest: pullRequestOn("github-personal", "robert"),
			want:        false,
		},
		{
			name:        "work instance, personal login → no match",
			pullRequest: pullRequestOn("github-work", "bob"),
			want:        false,
		},
		{
			name:        "unresolved third instance → no match",
			pullRequest: pullRequestOn("github-oss", "bob"),
			want:        false,
		},
	}

	for _, field := range []string{"author", "reviewer"} {
		t.Run(field, func(t *testing.T) {
			filterSpec := query.PRFilterSpec{}
			if field == "author" {
				filterSpec.Author = "me"
			} else {
				filterSpec.Reviewer = "me"
			}
			predicate, err := filterSpec.Compile(identities)
			if err != nil {
				t.Fatalf("Compile() error: %v", err)
			}

			for _, testCase := range tests {
				t.Run(testCase.name, func(t *testing.T) {
					if got := predicate(testCase.pullRequest); got != testCase.want {
						t.Errorf("predicate() = %v, want %v", got, testCase.want)
					}
				})
			}
		})
	}
}

func TestCompile_EmptySpec(t *testing.T) {
	predicate, err := query.PRFilterSpec{}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	// empty spec passes everything
	if !predicate(githubPR()) {
		t.Error("expected empty spec to pass all PRs")
	}
}
