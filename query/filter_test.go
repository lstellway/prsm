package query_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func boolPointer(value bool) *bool { return &value }

var resolvedMe = map[model.ProviderKind]model.Author{
	model.ProviderGitHub: {Username: "bob"},
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

	if !predicate(githubPR()) {
		t.Error("expected PR with no review state to match review_status=none")
	}
	if predicate(githubPR(withAggregateReview(model.AggregateReviewStateApproved))) {
		t.Error("expected approved PR to not match review_status=none")
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
	// resolvedMe has no entry for GitLab — "me" should not match any GitLab PR
	gitlabPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Kind: model.ProviderGitLab},
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
		Provider: model.ProviderInstance{Kind: model.ProviderGitLab},
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
