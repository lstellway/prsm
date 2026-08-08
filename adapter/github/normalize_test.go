package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// pointerTo returns a pointer to value. Used to construct go-github SDK structs that
// use pointer fields for all optional values.
func pointerTo[T any](value T) *T { return &value }

func makeCheckRun(status, conclusion string) *gogithub.CheckRun {
	checkRun := &gogithub.CheckRun{}
	if status != "" {
		checkRun.Status = pointerTo(status)
	}
	if conclusion != "" {
		checkRun.Conclusion = pointerTo(conclusion)
	}
	return checkRun
}

func makeReview(login, name, state string) *gogithub.PullRequestReview {
	return &gogithub.PullRequestReview{
		State: pointerTo(state),
		User: &gogithub.User{
			Login: pointerTo(login),
			Name:  pointerTo(name),
		},
	}
}

func makeResponse(headers map[string]string) *http.Response {
	header := http.Header{}
	for name, value := range headers {
		header.Set(name, value)
	}
	return &http.Response{Header: header}
}

// ---------------------------------------------------------------------------
// normalizeCIStatus
// ---------------------------------------------------------------------------

func TestNormalizeCIStatus(t *testing.T) {
	cases := []struct {
		name           string
		runs           []*gogithub.CheckRun
		wantState      model.CIState
		wantSummaryHas string
	}{
		{
			name:      "nil_runs",
			runs:      nil,
			wantState: model.CIStateNone,
		},
		{
			name:      "empty_runs",
			runs:      []*gogithub.CheckRun{},
			wantState: model.CIStateNone,
		},
		{
			name:           "all_success",
			runs:           []*gogithub.CheckRun{makeCheckRun("completed", "success"), makeCheckRun("completed", "success")},
			wantState:      model.CIStatePassing,
			wantSummaryHas: "2/2",
		},
		{
			name:           "one_failure",
			runs:           []*gogithub.CheckRun{makeCheckRun("completed", "success"), makeCheckRun("completed", "failure")},
			wantState:      model.CIStateFailing,
			wantSummaryHas: "1 failed",
		},
		{
			name:      "timed_out_counts_as_failure",
			runs:      []*gogithub.CheckRun{makeCheckRun("completed", "timed_out")},
			wantState: model.CIStateFailing,
		},
		{
			name:      "action_required_counts_as_failure",
			runs:      []*gogithub.CheckRun{makeCheckRun("completed", "action_required")},
			wantState: model.CIStateFailing,
		},
		{
			name:      "in_progress_counts_as_pending",
			runs:      []*gogithub.CheckRun{makeCheckRun("in_progress", "")},
			wantState: model.CIStatePending,
		},
		{
			name:      "queued_counts_as_pending",
			runs:      []*gogithub.CheckRun{makeCheckRun("queued", "")},
			wantState: model.CIStatePending,
		},
		{
			name:      "pending_beats_success",
			runs:      []*gogithub.CheckRun{makeCheckRun("completed", "success"), makeCheckRun("in_progress", "")},
			wantState: model.CIStatePending,
		},
		{
			name:      "failure_beats_pending",
			runs:      []*gogithub.CheckRun{makeCheckRun("in_progress", ""), makeCheckRun("completed", "failure")},
			wantState: model.CIStateFailing,
		},
		{
			name:           "summary_format_with_failures",
			runs:           []*gogithub.CheckRun{makeCheckRun("completed", "success"), makeCheckRun("completed", "success"), makeCheckRun("completed", "failure")},
			wantState:      model.CIStateFailing,
			wantSummaryHas: "2/3 checks passed, 1 failed",
		},
		{
			name:      "conclusion_case_insensitive",
			runs:      []*gogithub.CheckRun{makeCheckRun("COMPLETED", "SUCCESS")},
			wantState: model.CIStatePassing,
		},
		// DESIGN DECISION NEEDED: The default branch counts "skipped", "neutral",
		// "cancelled", and "stale" conclusions as passing. This may be wrong —
		// GitHub Actions treats "skipped" as a run that didn't execute, not a
		// successful one. The tests below encode the current behavior; update them
		// once the intended behavior is decided.
		{
			name:           "skipped_currently_counts_as_passing",
			runs:           []*gogithub.CheckRun{makeCheckRun("completed", "skipped")},
			wantState:      model.CIStatePassing,
			wantSummaryHas: "1/1",
		},
		{
			name:      "neutral_currently_counts_as_passing",
			runs:      []*gogithub.CheckRun{makeCheckRun("completed", "neutral")},
			wantState: model.CIStatePassing,
		},
		{
			name:      "cancelled_currently_counts_as_passing",
			runs:      []*gogithub.CheckRun{makeCheckRun("completed", "cancelled")},
			wantState: model.CIStatePassing,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := normalizeCIStatus(testCase.runs)
			if got.State != testCase.wantState {
				t.Errorf("State = %q, want %q", got.State, testCase.wantState)
			}
			if testCase.wantSummaryHas != "" && !strings.Contains(got.Summary, testCase.wantSummaryHas) {
				t.Errorf("Summary = %q, want it to contain %q", got.Summary, testCase.wantSummaryHas)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeAggregateState
// ---------------------------------------------------------------------------

func reviewerStateWith(decision model.ReviewDecision) model.ReviewerState {
	return model.ReviewerState{Decision: decision}
}

func TestNormalizeAggregateState(t *testing.T) {
	cases := []struct {
		name   string
		states []model.ReviewerState
		want   model.AggregateReviewState
	}{
		{"empty", nil, model.AggregateReviewStateNone},
		{"single_approved", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionApproved)}, model.AggregateReviewStateApproved},
		{"single_pending", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"single_commented", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionCommented)}, model.AggregateReviewStateCommented},
		{"single_changes_requested", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"changes_requested_beats_approved", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionApproved), reviewerStateWith(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"changes_requested_beats_pending", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionPending), reviewerStateWith(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"pending_beats_approved", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionApproved), reviewerStateWith(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"pending_beats_commented", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionCommented), reviewerStateWith(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"approved_beats_commented", []model.ReviewerState{reviewerStateWith(model.ReviewDecisionCommented), reviewerStateWith(model.ReviewDecisionApproved)}, model.AggregateReviewStateApproved},
		{
			"all_four_decisions",
			[]model.ReviewerState{reviewerStateWith(model.ReviewDecisionApproved), reviewerStateWith(model.ReviewDecisionCommented), reviewerStateWith(model.ReviewDecisionPending), reviewerStateWith(model.ReviewDecisionChangesRequested)},
			model.AggregateReviewStateChangesRequested,
		},
		// DESIGN DECISION NEEDED: ReviewDecisionDismissed is not handled in the
		// switch, so it falls through to the default and produces None. A dismissed
		// review could reasonably map to Pending (the reviewer has no active
		// decision) or be ignored. The test below encodes current behavior.
		{
			"dismissed_currently_produces_none",
			[]model.ReviewerState{reviewerStateWith(model.ReviewDecisionDismissed)},
			model.AggregateReviewStateNone,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := model.ComputeAggregateReviewState(testCase.states)
			if got != testCase.want {
				t.Errorf("ComputeAggregateReviewState = %q, want %q", got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeReviewerStates
// ---------------------------------------------------------------------------

func TestNormalizeReviewerStates(t *testing.T) {
	t.Run("nil_input_returns_nil", func(t *testing.T) {
		if got := normalizeReviewerStates(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty_input_returns_nil", func(t *testing.T) {
		if got := normalizeReviewerStates([]*gogithub.PullRequestReview{}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("skip_empty_login", func(t *testing.T) {
		if got := normalizeReviewerStates([]*gogithub.PullRequestReview{makeReview("", "Name", "APPROVED")}); got != nil {
			t.Errorf("expected nil for empty-login review, got %v", got)
		}
	})

	t.Run("single_approved", func(t *testing.T) {
		got := normalizeReviewerStates([]*gogithub.PullRequestReview{makeReview("alice", "Alice", "APPROVED")})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Reviewer.Username != "alice" {
			t.Errorf("Username = %q, want %q", got[0].Reviewer.Username, "alice")
		}
		if got[0].Decision != model.ReviewDecisionApproved {
			t.Errorf("Decision = %q, want Approved", got[0].Decision)
		}
	})

	t.Run("dedup_keeps_last_review", func(t *testing.T) {
		// alice approves then requests changes — should see ChangesRequested
		reviews := []*gogithub.PullRequestReview{
			makeReview("alice", "Alice", "APPROVED"),
			makeReview("alice", "Alice", "CHANGES_REQUESTED"),
		}
		got := normalizeReviewerStates(reviews)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Decision != model.ReviewDecisionChangesRequested {
			t.Errorf("Decision = %q, want ChangesRequested", got[0].Decision)
		}
	})

	t.Run("dedup_last_is_positional_not_worst", func(t *testing.T) {
		// alice requests changes then approves — should see Approved
		reviews := []*gogithub.PullRequestReview{
			makeReview("alice", "Alice", "CHANGES_REQUESTED"),
			makeReview("alice", "Alice", "APPROVED"),
		}
		got := normalizeReviewerStates(reviews)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Decision != model.ReviewDecisionApproved {
			t.Errorf("Decision = %q, want Approved", got[0].Decision)
		}
	})

	t.Run("multiple_reviewers_sorted_by_login", func(t *testing.T) {
		reviews := []*gogithub.PullRequestReview{
			makeReview("zara", "Zara", "APPROVED"),
			makeReview("alice", "Alice", "CHANGES_REQUESTED"),
			makeReview("bob", "Bob", "COMMENTED"),
		}
		got := normalizeReviewerStates(reviews)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		// Output must be sorted by username after the fix.
		if got[0].Reviewer.Username != "alice" || got[1].Reviewer.Username != "bob" || got[2].Reviewer.Username != "zara" {
			t.Errorf("order = [%s %s %s], want [alice bob zara]",
				got[0].Reviewer.Username, got[1].Reviewer.Username, got[2].Reviewer.Username)
		}
	})

	t.Run("name_falls_back_to_login", func(t *testing.T) {
		got := normalizeReviewerStates([]*gogithub.PullRequestReview{makeReview("alice", "", "APPROVED")})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Reviewer.DisplayName != "alice" {
			t.Errorf("DisplayName = %q, want %q", got[0].Reviewer.DisplayName, "alice")
		}
	})

	t.Run("name_present_used", func(t *testing.T) {
		got := normalizeReviewerStates([]*gogithub.PullRequestReview{makeReview("alice", "Alice Smith", "APPROVED")})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0].Reviewer.DisplayName != "Alice Smith" {
			t.Errorf("DisplayName = %q, want %q", got[0].Reviewer.DisplayName, "Alice Smith")
		}
	})
}

// ---------------------------------------------------------------------------
// checkRateLimit
// ---------------------------------------------------------------------------

func TestCheckRateLimit(t *testing.T) {
	instance := model.ProviderInstance{Host: "github.com"}

	t.Run("nil_response_returns_nil", func(t *testing.T) {
		if err := checkRateLimit(instance, nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("no_header_returns_nil", func(t *testing.T) {
		if err := checkRateLimit(instance, makeResponse(nil)); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("remaining_positive_returns_nil", func(t *testing.T) {
		response := makeResponse(map[string]string{"X-RateLimit-Remaining": "42"})
		if err := checkRateLimit(instance, response); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("remaining_zero_returns_error", func(t *testing.T) {
		response := makeResponse(map[string]string{"X-RateLimit-Remaining": "0"})
		err := checkRateLimit(instance, response)
		if err == nil {
			t.Fatal("expected RateLimitError, got nil")
		}
		var rateLimitErr adapter.RateLimitError
		if !errors.As(err, &rateLimitErr) {
			t.Fatalf("expected RateLimitError, got %T", err)
		}
		if rateLimitErr.Instance.Host != "github.com" {
			t.Errorf("Instance.Host = %q, want %q", rateLimitErr.Instance.Host, "github.com")
		}
	})

	t.Run("reset_header_sets_retry_after", func(t *testing.T) {
		resetTime := time.Now().Add(10 * time.Minute).Truncate(time.Second)
		response := makeResponse(map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     strconv.FormatInt(resetTime.Unix(), 10),
		})
		var rateLimitErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, response), &rateLimitErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rateLimitErr.RetryAfter.Equal(time.Unix(resetTime.Unix(), 0)) {
			t.Errorf("RetryAfter = %v, want %v", rateLimitErr.RetryAfter, resetTime)
		}
	})

	t.Run("retry_after_header_used_when_no_reset", func(t *testing.T) {
		response := makeResponse(map[string]string{
			"X-RateLimit-Remaining": "0",
			"Retry-After":           "30",
		})
		before := time.Now()
		var rateLimitErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, response), &rateLimitErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rateLimitErr.RetryAfter.After(before) {
			t.Errorf("RetryAfter %v should be after call time %v", rateLimitErr.RetryAfter, before)
		}
		if !rateLimitErr.RetryAfter.Before(before.Add(60 * time.Second)) {
			t.Errorf("RetryAfter %v is suspiciously far in the future", rateLimitErr.RetryAfter)
		}
	})

	t.Run("reset_takes_priority_over_retry_after", func(t *testing.T) {
		resetTime := time.Now().Add(5 * time.Minute)
		response := makeResponse(map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     strconv.FormatInt(resetTime.Unix(), 10),
			"Retry-After":           "999",
		})
		var rateLimitErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, response), &rateLimitErr) {
			t.Fatal("expected RateLimitError")
		}
		if rateLimitErr.RetryAfter.Equal(time.Unix(resetTime.Unix(), 0)) == false {
			t.Errorf("RetryAfter = %v; expected X-RateLimit-Reset value %v to take priority", rateLimitErr.RetryAfter, resetTime)
		}
	})

	t.Run("malformed_remaining_header_returns_nil", func(t *testing.T) {
		response := makeResponse(map[string]string{"X-RateLimit-Remaining": "notanumber"})
		if err := checkRateLimit(instance, response); err != nil {
			t.Errorf("expected nil for malformed header, got %v", err)
		}
	})

	t.Run("remaining_zero_no_reset_headers", func(t *testing.T) {
		response := makeResponse(map[string]string{"X-RateLimit-Remaining": "0"})
		var rateLimitErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, response), &rateLimitErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rateLimitErr.RetryAfter.IsZero() {
			t.Errorf("RetryAfter should be zero when no reset headers present, got %v", rateLimitErr.RetryAfter)
		}
	})
}

// ---------------------------------------------------------------------------
// normalizePRState
// ---------------------------------------------------------------------------

func TestNormalizePRState(t *testing.T) {
	cases := []struct {
		state    string
		isDraft  bool
		isMerged bool
		want     model.PRState
	}{
		{"OPEN", false, false, model.PRStateOpen},
		{"CLOSED", false, false, model.PRStateClosed},
		// GitHub list endpoint returns state="closed" with merged_at set for merged PRs.
		{"CLOSED", false, true, model.PRStateMerged},
		{"open", false, false, model.PRStateOpen},   // case-insensitive
		{"OPEN", true, false, model.PRStateDraft},   // draft overrides open state
		{"CLOSED", true, false, model.PRStateDraft}, // draft+closed (unreachable via GitHub API)
		{"OPEN", false, true, model.PRStateMerged},  // isMerged wins over all states
		{"UNKNOWN", false, false, model.PRStateOpen},
		{"", false, false, model.PRStateOpen},
	}
	for _, testCase := range cases {
		name := testCase.state + "_draft=" + strconv.FormatBool(testCase.isDraft) + "_merged=" + strconv.FormatBool(testCase.isMerged)
		t.Run(name, func(t *testing.T) {
			got := normalizePRState(testCase.state, testCase.isDraft, testCase.isMerged)
			if got != testCase.want {
				t.Errorf("normalizePRState(%q, %v, %v) = %q, want %q", testCase.state, testCase.isDraft, testCase.isMerged, got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeReviewDecision
// ---------------------------------------------------------------------------

func TestNormalizeReviewDecision(t *testing.T) {
	cases := []struct {
		input string
		want  model.ReviewDecision
	}{
		{"APPROVED", model.ReviewDecisionApproved},
		{"CHANGES_REQUESTED", model.ReviewDecisionChangesRequested},
		{"COMMENTED", model.ReviewDecisionCommented},
		{"DISMISSED", model.ReviewDecisionDismissed},
		{"approved", model.ReviewDecisionApproved}, // case-insensitive
		{"", model.ReviewDecisionPending},
		{"REVIEW_REQUESTED", model.ReviewDecisionPending},
	}
	for _, testCase := range cases {
		t.Run(testCase.input, func(t *testing.T) {
			got := normalizeReviewDecision(testCase.input)
			if got != testCase.want {
				t.Errorf("normalizeReviewDecision(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeLabels
// ---------------------------------------------------------------------------

func TestNormalizeLabels(t *testing.T) {
	t.Run("nil_returns_nil", func(t *testing.T) {
		if got := normalizeLabels(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("empty_returns_nil", func(t *testing.T) {
		if got := normalizeLabels([]*gogithub.Label{}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("color_without_hash_gets_prefix", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: pointerTo("bug"), Color: pointerTo("0075ca")}})
		if got[0].Color != "#0075ca" {
			t.Errorf("Color = %q, want %q", got[0].Color, "#0075ca")
		}
	})
	t.Run("color_with_hash_unchanged", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: pointerTo("bug"), Color: pointerTo("#0075ca")}})
		if got[0].Color != "#0075ca" {
			t.Errorf("Color = %q, want %q", got[0].Color, "#0075ca")
		}
	})
	t.Run("empty_color_unchanged", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: pointerTo("bug"), Color: pointerTo("")}})
		if got[0].Color != "" {
			t.Errorf("Color = %q, want empty", got[0].Color)
		}
	})
	t.Run("multiple_labels_order_preserved", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: pointerTo("a")}, {Name: pointerTo("b")}})
		if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
			t.Errorf("unexpected labels: %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// normalizeIdentity
// ---------------------------------------------------------------------------

func TestNormalizeIdentity(t *testing.T) {
	t.Run("name_present", func(t *testing.T) {
		got := normalizeIdentity("alice", "Alice Smith", "https://example.com/avatar.png")
		if got.Username != "alice" || got.DisplayName != "Alice Smith" || got.AvatarURL != "https://example.com/avatar.png" {
			t.Errorf("unexpected identity: %+v", got)
		}
	})
	t.Run("name_empty_falls_back_to_login", func(t *testing.T) {
		got := normalizeIdentity("alice", "", "")
		if got.DisplayName != "alice" {
			t.Errorf("DisplayName = %q, want %q", got.DisplayName, "alice")
		}
	})
	t.Run("login_always_set_as_username", func(t *testing.T) {
		got := normalizeIdentity("alice", "Alice", "")
		if got.Username != "alice" {
			t.Errorf("Username = %q, want %q", got.Username, "alice")
		}
	})
}

// ---------------------------------------------------------------------------
// normalizePR
// ---------------------------------------------------------------------------

func TestNormalizePR(t *testing.T) {
	instance := model.ProviderInstance{
		Name: "gh-personal",
		Kind: model.ProviderGitHub,
		Host: "github.com",
	}

	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	mergedTime := time.Date(2024, 1, 17, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		pullRequest *gogithub.PullRequest
		check       func(t *testing.T, got model.PullRequest)
	}{
		{
			name: "fully_populated",
			pullRequest: &gogithub.PullRequest{
				NodeID:  pointerTo("PR_abc123"),
				Number:  pointerTo(42),
				Title:   pointerTo("Add feature X"),
				HTMLURL: pointerTo("https://github.com/acme/repo/pull/42"),
				Body:    pointerTo("This PR adds feature X"),
				Head: &gogithub.PullRequestBranch{
					Ref: pointerTo("feature/x"),
					SHA: pointerTo("deadbeef"),
				},
				Base: &gogithub.PullRequestBranch{
					Ref: pointerTo("main"),
				},
				User: &gogithub.User{
					Login:     pointerTo("alice"),
					AvatarURL: pointerTo("https://avatars.githubusercontent.com/alice"),
				},
				Labels: []*gogithub.Label{
					{Name: pointerTo("bug"), Color: pointerTo("d73a4a")},
				},
				State:     pointerTo("open"),
				Draft:     pointerTo(false),
				CreatedAt: &gogithub.Timestamp{Time: baseTime},
				UpdatedAt: &gogithub.Timestamp{Time: updatedTime},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if got.ProviderID != "PR_abc123" {
					t.Errorf("ProviderID = %q, want %q", got.ProviderID, "PR_abc123")
				}
				if got.Number != 42 {
					t.Errorf("Number = %d, want 42", got.Number)
				}
				if got.Title != "Add feature X" {
					t.Errorf("Title = %q, want %q", got.Title, "Add feature X")
				}
				if got.URL != "https://github.com/acme/repo/pull/42" {
					t.Errorf("URL = %q", got.URL)
				}
				if got.Body != "This PR adds feature X" {
					t.Errorf("Body = %q", got.Body)
				}
				if got.SourceBranch != "feature/x" {
					t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, "feature/x")
				}
				if got.TargetBranch != "main" {
					t.Errorf("TargetBranch = %q, want %q", got.TargetBranch, "main")
				}
				if got.HeadSHA != "deadbeef" {
					t.Errorf("HeadSHA = %q, want %q", got.HeadSHA, "deadbeef")
				}
				if got.Author.Username != "alice" {
					t.Errorf("Author.Username = %q, want %q", got.Author.Username, "alice")
				}
				if len(got.Labels) != 1 {
					t.Fatalf("len(Labels) = %d, want 1", len(got.Labels))
				}
				if got.CreatedAt != baseTime {
					t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, baseTime)
				}
				if got.UpdatedAt != updatedTime {
					t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, updatedTime)
				}
				if got.Repo.Owner != "acme" {
					t.Errorf("Repo.Owner = %q, want %q", got.Repo.Owner, "acme")
				}
				if got.Repo.Name != "repo" {
					t.Errorf("Repo.Name = %q, want %q", got.Repo.Name, "repo")
				}
				if got.Provider.Kind != model.ProviderGitHub {
					t.Errorf("Provider.Kind = %q, want %q", got.Provider.Kind, model.ProviderGitHub)
				}
				if got.Provider.Name != "gh-personal" {
					t.Errorf("Provider.Name = %q, want %q", got.Provider.Name, "gh-personal")
				}
				if !got.CI.IsPending() {
					t.Errorf("CI should be Pending")
				}
				if !got.Diff.IsPending() {
					t.Errorf("Diff should be Pending")
				}
				if !got.Reviews.ReviewerStates.IsPending() {
					t.Errorf("Reviews.ReviewerStates should be Pending")
				}
				if got.Reviews.AggregateState != model.AggregateReviewStateNone {
					t.Errorf("AggregateState = %q, want None (no requested reviewers)", got.Reviews.AggregateState)
				}
			},
		},
		{
			name: "draft_pr",
			pullRequest: &gogithub.PullRequest{
				State: pointerTo("open"),
				Draft: pointerTo(true),
				Head:  &gogithub.PullRequestBranch{},
				Base:  &gogithub.PullRequestBranch{},
				User:  &gogithub.User{},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if got.State != model.PRStateDraft {
					t.Errorf("State = %q, want %q", got.State, model.PRStateDraft)
				}
			},
		},
		{
			name: "merged_pr",
			pullRequest: &gogithub.PullRequest{
				State:    pointerTo("closed"),
				Draft:    pointerTo(false),
				MergedAt: &gogithub.Timestamp{Time: mergedTime},
				Head:     &gogithub.PullRequestBranch{},
				Base:     &gogithub.PullRequestBranch{},
				User:     &gogithub.User{},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if got.State != model.PRStateMerged {
					t.Errorf("State = %q, want %q", got.State, model.PRStateMerged)
				}
				if got.MergedAt == nil {
					t.Fatal("MergedAt should be non-nil for a merged PR")
				}
				if !got.MergedAt.Equal(mergedTime) {
					t.Errorf("MergedAt = %v, want %v", *got.MergedAt, mergedTime)
				}
			},
		},
		{
			name: "open_pr_no_merged_at",
			pullRequest: &gogithub.PullRequest{
				State: pointerTo("open"),
				Draft: pointerTo(false),
				Head:  &gogithub.PullRequestBranch{},
				Base:  &gogithub.PullRequestBranch{},
				User:  &gogithub.User{},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if got.State != model.PRStateOpen {
					t.Errorf("State = %q, want %q", got.State, model.PRStateOpen)
				}
				if got.MergedAt != nil {
					t.Errorf("MergedAt should be nil for open PR, got %v", got.MergedAt)
				}
			},
		},
		{
			name: "with_requested_reviewers",
			pullRequest: &gogithub.PullRequest{
				State: pointerTo("open"),
				Draft: pointerTo(false),
				Head:  &gogithub.PullRequestBranch{},
				Base:  &gogithub.PullRequestBranch{},
				User:  &gogithub.User{},
				RequestedReviewers: []*gogithub.User{
					{Login: pointerTo("bob"), AvatarURL: pointerTo("https://avatars.githubusercontent.com/bob")},
					{Login: pointerTo("carol"), AvatarURL: pointerTo("https://avatars.githubusercontent.com/carol")},
				},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if len(got.Reviews.RequestedReviewers) != 2 {
					t.Fatalf("len(RequestedReviewers) = %d, want 2", len(got.Reviews.RequestedReviewers))
				}
				for index, requestedReviewer := range got.Reviews.RequestedReviewers {
					if requestedReviewer.Decision != model.ReviewDecisionPending {
						t.Errorf("RequestedReviewers[%d].Decision = %q, want Pending", index, requestedReviewer.Decision)
					}
				}
				if got.Reviews.AggregateState != model.AggregateReviewStateRequired {
					t.Errorf("AggregateState = %q, want %q", got.Reviews.AggregateState, model.AggregateReviewStateRequired)
				}
			},
		},
		{
			name: "no_requested_reviewers",
			pullRequest: &gogithub.PullRequest{
				State:              pointerTo("open"),
				Draft:              pointerTo(false),
				Head:               &gogithub.PullRequestBranch{},
				Base:               &gogithub.PullRequestBranch{},
				User:               &gogithub.User{},
				RequestedReviewers: nil,
			},
			check: func(t *testing.T, got model.PullRequest) {
				if got.Reviews.RequestedReviewers != nil {
					t.Errorf("RequestedReviewers should be nil, got %v", got.Reviews.RequestedReviewers)
				}
				if got.Reviews.AggregateState != model.AggregateReviewStateNone {
					t.Errorf("AggregateState = %q, want None", got.Reviews.AggregateState)
				}
			},
		},
		{
			name: "label_color_gets_hash_prefix",
			pullRequest: &gogithub.PullRequest{
				State: pointerTo("open"),
				Draft: pointerTo(false),
				Head:  &gogithub.PullRequestBranch{},
				Base:  &gogithub.PullRequestBranch{},
				User:  &gogithub.User{},
				Labels: []*gogithub.Label{
					{Name: pointerTo("urgent"), Color: pointerTo("ff0000")},
				},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if len(got.Labels) != 1 {
					t.Fatalf("len(Labels) = %d, want 1", len(got.Labels))
				}
				if got.Labels[0].Color != "#ff0000" {
					t.Errorf("Label.Color = %q, want %q", got.Labels[0].Color, "#ff0000")
				}
			},
		},
		{
			name: "lazy_fields_are_pending",
			pullRequest: &gogithub.PullRequest{
				State: pointerTo("open"),
				Draft: pointerTo(false),
				Head:  &gogithub.PullRequestBranch{},
				Base:  &gogithub.PullRequestBranch{},
				User:  &gogithub.User{},
			},
			check: func(t *testing.T, got model.PullRequest) {
				if !got.CI.IsPending() {
					t.Errorf("CI.IsPending() = false, want true")
				}
				if !got.Diff.IsPending() {
					t.Errorf("Diff.IsPending() = false, want true")
				}
				if !got.Reviews.ReviewerStates.IsPending() {
					t.Errorf("Reviews.ReviewerStates.IsPending() = false, want true")
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := normalizePR(testCase.pullRequest, "acme", "repo", instance)
			testCase.check(t, got)
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeReviewSummary
// ---------------------------------------------------------------------------

func TestNormalizeReviewSummary(t *testing.T) {
	cases := []struct {
		name      string
		reviewers []*gogithub.User
		check     func(t *testing.T, got model.ReviewSummary)
	}{
		{
			name:      "nil_reviewers",
			reviewers: nil,
			check: func(t *testing.T, got model.ReviewSummary) {
				if !got.ReviewerStates.IsPending() {
					t.Errorf("ReviewerStates.IsPending() = false, want true")
				}
				if got.AggregateState != model.AggregateReviewStateNone {
					t.Errorf("AggregateState = %q, want None", got.AggregateState)
				}
				if got.RequestedReviewers != nil {
					t.Errorf("RequestedReviewers should be nil, got %v", got.RequestedReviewers)
				}
			},
		},
		{
			name:      "empty_reviewers",
			reviewers: []*gogithub.User{},
			check: func(t *testing.T, got model.ReviewSummary) {
				if !got.ReviewerStates.IsPending() {
					t.Errorf("ReviewerStates.IsPending() = false, want true")
				}
				if got.AggregateState != model.AggregateReviewStateNone {
					t.Errorf("AggregateState = %q, want None", got.AggregateState)
				}
				if got.RequestedReviewers != nil {
					t.Errorf("RequestedReviewers should be nil, got %v", got.RequestedReviewers)
				}
			},
		},
		{
			name: "single_reviewer",
			reviewers: []*gogithub.User{
				{Login: pointerTo("bob"), AvatarURL: pointerTo("https://avatars.githubusercontent.com/bob")},
			},
			check: func(t *testing.T, got model.ReviewSummary) {
				if len(got.RequestedReviewers) != 1 {
					t.Fatalf("len(RequestedReviewers) = %d, want 1", len(got.RequestedReviewers))
				}
				requestedReviewer := got.RequestedReviewers[0]
				if requestedReviewer.Reviewer.Username != "bob" {
					t.Errorf("Username = %q, want %q", requestedReviewer.Reviewer.Username, "bob")
				}
				if requestedReviewer.Reviewer.AvatarURL != "https://avatars.githubusercontent.com/bob" {
					t.Errorf("AvatarURL = %q", requestedReviewer.Reviewer.AvatarURL)
				}
				if requestedReviewer.Decision != model.ReviewDecisionPending {
					t.Errorf("Decision = %q, want Pending", requestedReviewer.Decision)
				}
				if got.AggregateState != model.AggregateReviewStateRequired {
					t.Errorf("AggregateState = %q, want Required", got.AggregateState)
				}
				if !got.ReviewerStates.IsPending() {
					t.Errorf("ReviewerStates.IsPending() = false, want true")
				}
			},
		},
		{
			name: "reviewer_empty_login_skipped",
			reviewers: []*gogithub.User{
				{Login: pointerTo(""), AvatarURL: pointerTo("https://avatars.githubusercontent.com/ghost")},
			},
			check: func(t *testing.T, got model.ReviewSummary) {
				if got.RequestedReviewers != nil {
					t.Errorf("RequestedReviewers should be nil for empty-login reviewer, got %v", got.RequestedReviewers)
				}
			},
		},
		{
			name: "multiple_reviewers",
			reviewers: []*gogithub.User{
				{Login: pointerTo("alice")},
				{Login: pointerTo("bob")},
				{Login: pointerTo("carol")},
			},
			check: func(t *testing.T, got model.ReviewSummary) {
				if len(got.RequestedReviewers) != 3 {
					t.Fatalf("len(RequestedReviewers) = %d, want 3", len(got.RequestedReviewers))
				}
				if got.AggregateState != model.AggregateReviewStateRequired {
					t.Errorf("AggregateState = %q, want Required", got.AggregateState)
				}
			},
		},
		{
			name: "display_name_falls_back_to_login",
			reviewers: []*gogithub.User{
				{Login: pointerTo("dave")},
			},
			check: func(t *testing.T, got model.ReviewSummary) {
				if len(got.RequestedReviewers) != 1 {
					t.Fatalf("len(RequestedReviewers) = %d, want 1", len(got.RequestedReviewers))
				}
				requestedReviewer := got.RequestedReviewers[0]
				if requestedReviewer.Reviewer.DisplayName != "dave" {
					t.Errorf("DisplayName = %q, want %q (should fall back to login)", requestedReviewer.Reviewer.DisplayName, "dave")
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := normalizeReviewSummary(testCase.reviewers)
			testCase.check(t, got)
		})
	}
}
