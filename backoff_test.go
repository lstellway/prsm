package prsm

import (
	"fmt"
	"testing"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

func TestBackoffPolicy_Wait(t *testing.T) {
	policy := BackoffPolicy{Initial: 10 * time.Second, Max: 100 * time.Second, Multiplier: 2}

	testCases := []struct {
		name         string
		failureCount int
		want         time.Duration
	}{
		{"first failure", 1, 10 * time.Second},
		{"second failure doubles", 2, 20 * time.Second},
		{"third failure doubles again", 3, 40 * time.Second},
		{"fourth failure doubles again", 4, 80 * time.Second},
		{"fifth failure capped at Max", 5, 100 * time.Second},
		{"tenth failure still capped", 10, 100 * time.Second},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := policy.wait(testCase.failureCount); got != testCase.want {
				t.Errorf("wait(%d) = %v, want %v", testCase.failureCount, got, testCase.want)
			}
		})
	}
}

func TestDefaultBackoffPolicy(t *testing.T) {
	policy := DefaultBackoffPolicy()
	if policy.Initial <= 0 || policy.Max <= 0 || policy.Multiplier <= 1 || policy.OfflineAfter <= 0 {
		t.Errorf("DefaultBackoffPolicy() = %+v, want all positive fields with Multiplier > 1", policy)
	}
}

func TestRateLimitBucketKey(t *testing.T) {
	instance := func(name, account string) model.ProviderInstance {
		return model.ProviderInstance{Name: name, Kind: model.ProviderGitHub, Host: "github.com", Account: account}
	}

	// Two connections with different Names but the same Kind+Host+Account
	// share one bucket.
	keyOne := rateLimitBucketKey(instance("work-token", "alice"))
	keyTwo := rateLimitBucketKey(instance("personal-token", "alice"))
	if keyOne != keyTwo {
		t.Errorf("keys for two connections sharing an account = %q, %q, want equal", keyOne, keyTwo)
	}

	// A different account on the same host does not share a bucket.
	keyBob := rateLimitBucketKey(instance("bobs-token", "bob"))
	if keyOne == keyBob {
		t.Errorf("keys for different accounts on the same host = %q, %q, want distinct", keyOne, keyBob)
	}

	// Two connections with empty Account (not yet resolved) and different
	// Names must NOT collapse into one bucket.
	unresolvedOne := rateLimitBucketKey(instance("conn-one", ""))
	unresolvedTwo := rateLimitBucketKey(instance("conn-two", ""))
	if unresolvedOne == unresolvedTwo {
		t.Errorf("keys for two unresolved connections with different names = %q, %q, want distinct", unresolvedOne, unresolvedTwo)
	}

	// A Name-fallback key must never collide with an Account key, even if
	// the literal strings happen to match.
	nameFallbackKey := rateLimitBucketKey(instance("alice", ""))
	accountKey := rateLimitBucketKey(instance("conn-x", "alice"))
	if nameFallbackKey == accountKey {
		t.Errorf("name-fallback key collided with an account key: %q", nameFallbackKey)
	}
}

func TestRateLimitBucket(t *testing.T) {
	now := time.Now()
	bucket := &rateLimitBucket{}

	if !bucket.due(now) {
		t.Error("zero-value bucket due(now) = false, want true")
	}

	bucket.hold(now.Add(time.Minute))
	if bucket.due(now) {
		t.Error("bucket due(now) = true after hold(now+1m), want false")
	}
	if !bucket.due(now.Add(2 * time.Minute)) {
		t.Error("bucket due(now+2m) = false after hold(now+1m), want true")
	}

	// hold never shortens an existing hold.
	bucket.hold(now.Add(30 * time.Second))
	if bucket.due(now.Add(45 * time.Second)) {
		t.Error("hold() shortened an existing later hold; due(now+45s) = true, want false (still held until now+1m)")
	}
}

func TestRateLimitRetryAfter(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name string
		err  error
		want time.Time
	}{
		{"explicit RetryAfter", adapter.RateLimitError{RetryAfter: now.Add(5 * time.Minute)}, now.Add(5 * time.Minute)},
		{"zero RetryAfter defaults", adapter.RateLimitError{}, now.Add(defaultRateLimitBackoff)},
		{"wrapped RateLimitError", fmt.Errorf("wrapped: %w", adapter.RateLimitError{RetryAfter: now.Add(time.Hour)}), now.Add(time.Hour)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := rateLimitRetryAfter(testCase.err, now); !got.Equal(testCase.want) {
				t.Errorf("rateLimitRetryAfter() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestConnectionSchedule_RecordSuccess(t *testing.T) {
	schedule := &connectionSchedule[string]{
		consecutiveFailures: 3,
		nextAttemptAt:       time.Now().Add(time.Minute),
		lastItems:           []string{"stale"},
	}
	status := ConnectionStatus{connectionOutcome: connectionOutcome{State: ConnectionStateOK}}

	schedule.recordSuccess([]string{"fresh"}, status)

	if schedule.consecutiveFailures != 0 {
		t.Errorf("consecutiveFailures = %d, want 0", schedule.consecutiveFailures)
	}
	if !schedule.nextAttemptAt.IsZero() {
		t.Errorf("nextAttemptAt = %v, want zero", schedule.nextAttemptAt)
	}
	if len(schedule.lastItems) != 1 || schedule.lastItems[0] != "fresh" {
		t.Errorf("lastItems = %v, want [fresh]", schedule.lastItems)
	}
	if schedule.lastStatus.State != ConnectionStateOK {
		t.Errorf("lastStatus.State = %v, want ConnectionStateOK", schedule.lastStatus.State)
	}
}

func TestConnectionSchedule_RecordFailureKeepsLastItems(t *testing.T) {
	schedule := &connectionSchedule[string]{lastItems: []string{"kept"}}
	policy := BackoffPolicy{Initial: 10 * time.Second, Max: time.Minute, Multiplier: 2}
	now := time.Now()

	failureStatus := ConnectionStatus{connectionOutcome: connectionOutcome{State: ConnectionStateOffline}}
	schedule.recordFailure(now, policy, failureStatus)

	if schedule.consecutiveFailures != 1 {
		t.Errorf("consecutiveFailures = %d, want 1", schedule.consecutiveFailures)
	}
	if want := now.Add(10 * time.Second); !schedule.nextAttemptAt.Equal(want) {
		t.Errorf("nextAttemptAt = %v, want %v", schedule.nextAttemptAt, want)
	}
	if len(schedule.lastItems) != 1 || schedule.lastItems[0] != "kept" {
		t.Errorf("lastItems = %v, want unchanged [kept] — a failed attempt must not erase last-known items", schedule.lastItems)
	}
}

func TestConnectionSchedule_RecordRateLimitedLeavesStreakAndItems(t *testing.T) {
	nextAttemptAt := time.Now().Add(time.Minute)
	schedule := &connectionSchedule[string]{
		consecutiveFailures: 2,
		nextAttemptAt:       nextAttemptAt,
		lastItems:           []string{"kept"},
	}

	rateLimitedStatus := ConnectionStatus{connectionOutcome: connectionOutcome{State: ConnectionStateRateLimited}}
	schedule.recordRateLimited(rateLimitedStatus)

	if schedule.consecutiveFailures != 2 {
		t.Errorf("consecutiveFailures = %d, want unchanged 2 — a rate limit is not evidence of an unhealthy connection", schedule.consecutiveFailures)
	}
	if !schedule.nextAttemptAt.Equal(nextAttemptAt) {
		t.Errorf("nextAttemptAt = %v, want unchanged %v — rate-limit scheduling is driven by the shared bucket instead", schedule.nextAttemptAt, nextAttemptAt)
	}
	if len(schedule.lastItems) != 1 || schedule.lastItems[0] != "kept" {
		t.Errorf("lastItems = %v, want unchanged [kept]", schedule.lastItems)
	}
	if schedule.lastStatus.State != ConnectionStateRateLimited {
		t.Errorf("lastStatus.State = %v, want ConnectionStateRateLimited", schedule.lastStatus.State)
	}
}

func TestConnectionSchedule_Offline(t *testing.T) {
	policy := BackoffPolicy{OfflineAfter: 3}
	schedule := &connectionSchedule[int]{}

	for failureCount := 1; failureCount <= 2; failureCount++ {
		schedule.consecutiveFailures = failureCount
		if schedule.offline(policy) {
			t.Errorf("offline() = true at %d consecutive failures, want false (threshold 3)", failureCount)
		}
	}

	schedule.consecutiveFailures = 3
	if !schedule.offline(policy) {
		t.Error("offline() = false at 3 consecutive failures, want true")
	}
}
