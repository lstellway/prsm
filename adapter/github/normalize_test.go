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

// ptr returns a pointer to v. Used to construct go-github SDK structs that use
// pointer fields for all optional values.
func ptr[T any](v T) *T { return &v }

func makeCheckRun(status, conclusion string) *gogithub.CheckRun {
	r := &gogithub.CheckRun{}
	if status != "" {
		r.Status = ptr(status)
	}
	if conclusion != "" {
		r.Conclusion = ptr(conclusion)
	}
	return r
}

func makeReview(login, name, state string) *gogithub.PullRequestReview {
	return &gogithub.PullRequestReview{
		State: ptr(state),
		User: &gogithub.User{
			Login: ptr(login),
			Name:  ptr(name),
		},
	}
}

func makeResp(headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{Header: h}
}

// ---------------------------------------------------------------------------
// normalizeCIStatus
// ---------------------------------------------------------------------------

func TestNormalizeCIStatus(t *testing.T) {
	cases := []struct {
		name            string
		runs            []*gogithub.CheckRun
		wantState       model.CIState
		wantSummaryHas  string
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeCIStatus(tc.runs)
			if got.State != tc.wantState {
				t.Errorf("State = %q, want %q", got.State, tc.wantState)
			}
			if tc.wantSummaryHas != "" && !strings.Contains(got.Summary, tc.wantSummaryHas) {
				t.Errorf("Summary = %q, want it to contain %q", got.Summary, tc.wantSummaryHas)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// normalizeAggregateState
// ---------------------------------------------------------------------------

func rs(decision model.ReviewDecision) model.ReviewerState {
	return model.ReviewerState{Decision: decision}
}

func TestNormalizeAggregateState(t *testing.T) {
	cases := []struct {
		name   string
		states []model.ReviewerState
		want   model.AggregateReviewState
	}{
		{"empty", nil, model.AggregateReviewStateNone},
		{"single_approved", []model.ReviewerState{rs(model.ReviewDecisionApproved)}, model.AggregateReviewStateApproved},
		{"single_pending", []model.ReviewerState{rs(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"single_commented", []model.ReviewerState{rs(model.ReviewDecisionCommented)}, model.AggregateReviewStateCommented},
		{"single_changes_requested", []model.ReviewerState{rs(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"changes_requested_beats_approved", []model.ReviewerState{rs(model.ReviewDecisionApproved), rs(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"changes_requested_beats_pending", []model.ReviewerState{rs(model.ReviewDecisionPending), rs(model.ReviewDecisionChangesRequested)}, model.AggregateReviewStateChangesRequested},
		{"pending_beats_approved", []model.ReviewerState{rs(model.ReviewDecisionApproved), rs(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"pending_beats_commented", []model.ReviewerState{rs(model.ReviewDecisionCommented), rs(model.ReviewDecisionPending)}, model.AggregateReviewStateRequired},
		{"approved_beats_commented", []model.ReviewerState{rs(model.ReviewDecisionCommented), rs(model.ReviewDecisionApproved)}, model.AggregateReviewStateApproved},
		{
			"all_four_decisions",
			[]model.ReviewerState{rs(model.ReviewDecisionApproved), rs(model.ReviewDecisionCommented), rs(model.ReviewDecisionPending), rs(model.ReviewDecisionChangesRequested)},
			model.AggregateReviewStateChangesRequested,
		},
		// DESIGN DECISION NEEDED: ReviewDecisionDismissed is not handled in the
		// switch, so it falls through to the default and produces None. A dismissed
		// review could reasonably map to Pending (the reviewer has no active
		// decision) or be ignored. The test below encodes current behavior.
		{
			"dismissed_currently_produces_none",
			[]model.ReviewerState{rs(model.ReviewDecisionDismissed)},
			model.AggregateReviewStateNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeAggregateState(tc.states)
			if got != tc.want {
				t.Errorf("normalizeAggregateState = %q, want %q", got, tc.want)
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
		if err := checkRateLimit(instance, makeResp(nil)); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("remaining_positive_returns_nil", func(t *testing.T) {
		resp := makeResp(map[string]string{"X-RateLimit-Remaining": "42"})
		if err := checkRateLimit(instance, resp); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("remaining_zero_returns_error", func(t *testing.T) {
		resp := makeResp(map[string]string{"X-RateLimit-Remaining": "0"})
		err := checkRateLimit(instance, resp)
		if err == nil {
			t.Fatal("expected RateLimitError, got nil")
		}
		var rlErr adapter.RateLimitError
		if !errors.As(err, &rlErr) {
			t.Fatalf("expected RateLimitError, got %T", err)
		}
		if rlErr.Instance.Host != "github.com" {
			t.Errorf("Instance.Host = %q, want %q", rlErr.Instance.Host, "github.com")
		}
	})

	t.Run("reset_header_sets_retry_after", func(t *testing.T) {
		resetTime := time.Now().Add(10 * time.Minute).Truncate(time.Second)
		resp := makeResp(map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     strconv.FormatInt(resetTime.Unix(), 10),
		})
		var rlErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, resp), &rlErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rlErr.RetryAfter.Equal(time.Unix(resetTime.Unix(), 0)) {
			t.Errorf("RetryAfter = %v, want %v", rlErr.RetryAfter, resetTime)
		}
	})

	t.Run("retry_after_header_used_when_no_reset", func(t *testing.T) {
		resp := makeResp(map[string]string{
			"X-RateLimit-Remaining": "0",
			"Retry-After":           "30",
		})
		before := time.Now()
		var rlErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, resp), &rlErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rlErr.RetryAfter.After(before) {
			t.Errorf("RetryAfter %v should be after call time %v", rlErr.RetryAfter, before)
		}
		if !rlErr.RetryAfter.Before(before.Add(60 * time.Second)) {
			t.Errorf("RetryAfter %v is suspiciously far in the future", rlErr.RetryAfter)
		}
	})

	t.Run("reset_takes_priority_over_retry_after", func(t *testing.T) {
		resetTime := time.Now().Add(5 * time.Minute)
		resp := makeResp(map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     strconv.FormatInt(resetTime.Unix(), 10),
			"Retry-After":           "999",
		})
		var rlErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, resp), &rlErr) {
			t.Fatal("expected RateLimitError")
		}
		if rlErr.RetryAfter.Equal(time.Unix(resetTime.Unix(), 0)) == false {
			t.Errorf("RetryAfter = %v; expected X-RateLimit-Reset value %v to take priority", rlErr.RetryAfter, resetTime)
		}
	})

	t.Run("malformed_remaining_header_returns_nil", func(t *testing.T) {
		resp := makeResp(map[string]string{"X-RateLimit-Remaining": "notanumber"})
		if err := checkRateLimit(instance, resp); err != nil {
			t.Errorf("expected nil for malformed header, got %v", err)
		}
	})

	t.Run("remaining_zero_no_reset_headers", func(t *testing.T) {
		resp := makeResp(map[string]string{"X-RateLimit-Remaining": "0"})
		var rlErr adapter.RateLimitError
		if !errors.As(checkRateLimit(instance, resp), &rlErr) {
			t.Fatal("expected RateLimitError")
		}
		if !rlErr.RetryAfter.IsZero() {
			t.Errorf("RetryAfter should be zero when no reset headers present, got %v", rlErr.RetryAfter)
		}
	})
}

// ---------------------------------------------------------------------------
// normalizePRState
// ---------------------------------------------------------------------------

func TestNormalizePRState(t *testing.T) {
	cases := []struct {
		state   string
		isDraft bool
		want    model.PRState
	}{
		{"OPEN", false, model.PRStateOpen},
		{"CLOSED", false, model.PRStateClosed},
		{"MERGED", false, model.PRStateMerged},
		{"open", false, model.PRStateOpen},   // case-insensitive
		{"OPEN", true, model.PRStateDraft},   // draft overrides state
		{"CLOSED", true, model.PRStateDraft}, // draft overrides even closed
		{"UNKNOWN", false, model.PRStateOpen},
		{"", false, model.PRStateOpen},
	}
	for _, tc := range cases {
		t.Run(tc.state+"_draft="+strconv.FormatBool(tc.isDraft), func(t *testing.T) {
			got := normalizePRState(tc.state, tc.isDraft)
			if got != tc.want {
				t.Errorf("normalizePRState(%q, %v) = %q, want %q", tc.state, tc.isDraft, got, tc.want)
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
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeReviewDecision(tc.input)
			if got != tc.want {
				t.Errorf("normalizeReviewDecision(%q) = %q, want %q", tc.input, got, tc.want)
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
		got := normalizeLabels([]*gogithub.Label{{Name: ptr("bug"), Color: ptr("0075ca")}})
		if got[0].Color != "#0075ca" {
			t.Errorf("Color = %q, want %q", got[0].Color, "#0075ca")
		}
	})
	t.Run("color_with_hash_unchanged", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: ptr("bug"), Color: ptr("#0075ca")}})
		if got[0].Color != "#0075ca" {
			t.Errorf("Color = %q, want %q", got[0].Color, "#0075ca")
		}
	})
	t.Run("empty_color_unchanged", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: ptr("bug"), Color: ptr("")}})
		if got[0].Color != "" {
			t.Errorf("Color = %q, want empty", got[0].Color)
		}
	})
	t.Run("multiple_labels_order_preserved", func(t *testing.T) {
		got := normalizeLabels([]*gogithub.Label{{Name: ptr("a")}, {Name: ptr("b")}})
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
