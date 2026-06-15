package query_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func boolPtr(b bool) *bool { return &b }

var resolvedMe = map[model.ProviderKind]model.Author{
	model.ProviderGitHub: {Username: "bob"},
}

func ghPR(opts ...func(*model.PullRequest)) model.PullRequest {
	pr := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub, Account: "acme"},
		State:    model.PRStateOpen,
	}
	for _, o := range opts {
		o(&pr)
	}
	return pr
}

func withReviewer(username string) func(*model.PullRequest) {
	return func(pr *model.PullRequest) {
		pr.Reviews.RequestedReviewers = append(pr.Reviews.RequestedReviewers,
			model.ReviewerState{Reviewer: model.Reviewer{Username: username}},
		)
	}
}

func withAuthor(username string) func(*model.PullRequest) {
	return func(pr *model.PullRequest) { pr.Author.Username = username }
}

func withState(s model.PRState) func(*model.PullRequest) {
	return func(pr *model.PullRequest) { pr.State = s }
}

func withLabels(names ...string) func(*model.PullRequest) {
	return func(pr *model.PullRequest) {
		for _, n := range names {
			pr.Labels = append(pr.Labels, model.Label{Name: n})
		}
	}
}

func withRepo(owner, name string) func(*model.PullRequest) {
	return func(pr *model.PullRequest) {
		pr.Repo = model.Repository{Owner: owner, Name: name}
	}
}

func withAggregateReview(s model.AggregateReviewState) func(*model.PullRequest) {
	return func(pr *model.PullRequest) { pr.Reviews.AggregateState = s }
}

func withTargetBranch(b string) func(*model.PullRequest) {
	return func(pr *model.PullRequest) { pr.TargetBranch = b }
}

// TestCompile_ReviewerMeAndNoDraft is the primary acceptance criterion:
// PRFilterSpec{Reviewer: "me", Draft: boolPtr(false)}.Compile(resolvedMe) must correctly
// filter a mixed list.
func TestCompile_ReviewerMeAndNoDraft(t *testing.T) {
	spec := query.PRFilterSpec{
		Reviewer: "me",
		Draft:    boolPtr(false),
	}
	pred, err := spec.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	tests := []struct {
		name string
		pr   model.PullRequest
		want bool
	}{
		{
			name: "open PR with bob as reviewer → pass",
			pr:   ghPR(withReviewer("bob")),
			want: true,
		},
		{
			name: "draft PR with bob as reviewer → fail (is draft)",
			pr:   ghPR(withReviewer("bob"), withState(model.PRStateDraft)),
			want: false,
		},
		{
			name: "open PR without bob as reviewer → fail",
			pr:   ghPR(withReviewer("alice")),
			want: false,
		},
		{
			name: "open PR with no reviewers → fail",
			pr:   ghPR(),
			want: false,
		},
		{
			name: "open PR with bob and alice as reviewers → pass",
			pr:   ghPR(withReviewer("alice"), withReviewer("bob")),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pred(tt.pr); got != tt.want {
				t.Errorf("pred() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompile_Author(t *testing.T) {
	pred, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withAuthor("bob"))) {
		t.Error("expected PR authored by bob to match author=me")
	}
	if pred(ghPR(withAuthor("alice"))) {
		t.Error("expected PR authored by alice to not match author=me")
	}
}

func TestCompile_LabelANDMatch(t *testing.T) {
	pred, err := query.PRFilterSpec{Label: []string{"bug", "priority"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withLabels("bug", "priority"))) {
		t.Error("expected PR with both labels to match")
	}
	if pred(ghPR(withLabels("bug"))) {
		t.Error("expected PR with only one label to not match")
	}
	if pred(ghPR()) {
		t.Error("expected PR with no labels to not match")
	}
}

func TestCompile_RepoORMatch(t *testing.T) {
	pred, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Repo: []string{"acme/api", "acme/frontend"}}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withRepo("acme", "api"))) {
		t.Error("expected acme/api to match")
	}
	if !pred(ghPR(withRepo("acme", "frontend"))) {
		t.Error("expected acme/frontend to match")
	}
	if pred(ghPR(withRepo("acme", "backend"))) {
		t.Error("expected acme/backend to not match")
	}
}

func TestCompile_State(t *testing.T) {
	pred, err := query.PRFilterSpec{State: "open"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withState(model.PRStateOpen))) {
		t.Error("expected open PR to match")
	}
	if pred(ghPR(withState(model.PRStateMerged))) {
		t.Error("expected merged PR to not match")
	}
}

func TestCompile_ReviewStatus(t *testing.T) {
	pred, err := query.PRFilterSpec{ReviewStatus: "review_required"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withAggregateReview(model.AggregateReviewStateRequired))) {
		t.Error("expected review_required PR to match")
	}
	if pred(ghPR(withAggregateReview(model.AggregateReviewStateApproved))) {
		t.Error("expected approved PR to not match")
	}
}

func TestCompile_ReviewStatus_None(t *testing.T) {
	pred, err := query.PRFilterSpec{ReviewStatus: "none"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR()) {
		t.Error("expected PR with no review state to match review_status=none")
	}
	if pred(ghPR(withAggregateReview(model.AggregateReviewStateApproved))) {
		t.Error("expected approved PR to not match review_status=none")
	}
}

func TestCompile_TargetBranch(t *testing.T) {
	pred, err := query.PRFilterSpec{TargetBranch: "main"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !pred(ghPR(withTargetBranch("main"))) {
		t.Error("expected PR targeting main to match")
	}
	if !pred(ghPR(withTargetBranch("release/main-2026"))) {
		t.Error("expected PR targeting release/main-2026 to match (substring)")
	}
	if pred(ghPR(withTargetBranch("develop"))) {
		t.Error("expected PR targeting develop to not match")
	}
}

func TestCompile_CIStatus_PendingPassthrough(t *testing.T) {
	pred, err := query.PRFilterSpec{CIStatus: "failing"}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	prPending := ghPR()
	// CI is LoadStatePending (zero value) — must pass through
	if !pred(prPending) {
		t.Error("expected pending CI PR to pass through ci_status filter (Option C)")
	}

	prFailing := ghPR()
	prFailing.CI = model.Loaded(model.CIStatus{State: model.CIStateFailing})
	if !pred(prFailing) {
		t.Error("expected failing CI PR to match ci_status=failing")
	}

	prPassing := ghPR()
	prPassing.CI = model.Loaded(model.CIStatus{State: model.CIStatePassing})
	if pred(prPassing) {
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
	pred, err := query.PRFilterSpec{
		BaseFilterSpec: query.BaseFilterSpec{Provider: []string{"github-personal", "gitlab-work"}},
	}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	prGitHub := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub},
		State:    model.PRStateOpen,
	}
	prGitLab := model.PullRequest{
		Provider: model.ProviderInstance{Name: "gitlab-work", Kind: model.ProviderGitLab},
		State:    model.PRStateOpen,
	}
	prOther := model.PullRequest{
		Provider: model.ProviderInstance{Name: "codeberg", Kind: model.ProviderGitea},
		State:    model.PRStateOpen,
	}

	if !pred(prGitHub) {
		t.Error("expected github-personal to match")
	}
	if !pred(prGitLab) {
		t.Error("expected gitlab-work to match")
	}
	if pred(prOther) {
		t.Error("expected codeberg to not match")
	}
}

func TestCompile_DraftBoolField(t *testing.T) {
	pred, err := query.PRFilterSpec{Draft: boolPtr(false)}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// Provider sets pr.Draft = true with State = open (e.g. GitHub before it adopted PRStateDraft)
	prDraftBool := ghPR()
	prDraftBool.State = model.PRStateOpen
	prDraftBool.Draft = true
	if pred(prDraftBool) {
		t.Error("expected PR with Draft=true to be excluded by draft=false filter")
	}

	// Draft=false, State=open → passes
	if !pred(ghPR()) {
		t.Error("expected open non-draft PR to pass draft=false filter")
	}
}

func TestCompile_ResolveMe_UnknownProvider(t *testing.T) {
	// resolvedMe has no entry for GitLab — "me" should not match any GitLab PR
	gitlabPR := model.PullRequest{
		Provider: model.ProviderInstance{Kind: model.ProviderGitLab},
		Author:   model.Author{Username: "bob"},
		State:    model.PRStateOpen,
	}

	pred, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if pred(gitlabPR) {
		t.Error("expected 'me' with unresolved provider to match nothing, not any GitLab PR")
	}
}

func TestCompile_ResolveMe_EmptyUsername(t *testing.T) {
	// A PR with an empty Author.Username must not match author="me" when "me" is unresolved
	prEmptyAuthor := model.PullRequest{
		Provider: model.ProviderInstance{Kind: model.ProviderGitLab},
		Author:   model.Author{Username: ""},
		State:    model.PRStateOpen,
	}

	pred, err := query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	if pred(prEmptyAuthor) {
		t.Error("unresolved 'me' must not match a PR with an empty author username")
	}
}

func TestCompile_EmptySpec(t *testing.T) {
	pred, err := query.PRFilterSpec{}.Compile(resolvedMe)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	// empty spec passes everything
	if !pred(ghPR()) {
		t.Error("expected empty spec to pass all PRs")
	}
}
