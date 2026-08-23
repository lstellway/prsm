package prsm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
)

func connectionStatusFor(snapshot PullRequestSnapshot, name string) (ConnectionStatus, bool) {
	for _, status := range snapshot.Connections {
		if status.Provider.Name == name {
			return status, true
		}
	}
	return ConnectionStatus{}, false
}

// newMockPullRequestSource builds a single named GitHub connection returning
// pullRequests or err, cutting the three-level mock.PullRequestSource{
// Connection: mock.Connection{InstanceVal: ...}} literal every Fetch test
// would otherwise repeat.
func newMockPullRequestSource(name string, pullRequests []model.PullRequest, err error) *mock.PullRequestSource {
	return &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: name, Kind: model.ProviderGitHub}},
		PullRequests:    pullRequests,
		PullRequestsErr: err,
	}
}

func TestNewConnectionStatus_Success(t *testing.T) {
	instance := model.ProviderInstance{Name: "instance", Kind: model.ProviderGitHub}
	succeededAt := time.Now()

	status := newConnectionStatus(instance, succeededAt, nil)

	if status.Provider != instance {
		t.Errorf("Provider = %+v, want %+v", status.Provider, instance)
	}
	if status.State != ConnectionStateOK {
		t.Errorf("State = %v, want ConnectionStateOK", status.State)
	}
	if !status.SucceededAt.Equal(succeededAt) {
		t.Errorf("SucceededAt = %v, want %v", status.SucceededAt, succeededAt)
	}
	if status.Err != nil {
		t.Errorf("Err = %v, want nil", status.Err)
	}
}

// TestNewConnectionStatus_Classification exercises newConnectionStatus
// directly rather than through a full Fetch call, so a new ConnectionState
// case only needs a new table row instead of a mock/client/goroutine round
// trip. sentinelTime stands in for whatever "this Fetch call" timestamp a
// caller happens to pass; classification never uses it on the error path, so
// every case asserts SucceededAt stays zero regardless.
func TestNewConnectionStatus_Classification(t *testing.T) {
	sentinelTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		err       error
		wantState ConnectionState
	}{
		{"plain error", errors.New("connection refused"), ConnectionStateOffline},
		{"rate limit error", adapter.RateLimitError{}, ConnectionStateRateLimited},
		{"auth error", adapter.AuthError{}, ConnectionStateUnauthorized},
		{"not found error falls back to offline", adapter.NotFoundError{}, ConnectionStateOffline},
		{"rate limit wrapped with fmt.Errorf", fmt.Errorf("listing pull requests: %w", adapter.RateLimitError{}), ConnectionStateRateLimited},
		{"rate limit inside errors.Join with a plain error", errors.Join(errors.New("network error"), adapter.RateLimitError{}), ConnectionStateRateLimited},
		{"auth error inside errors.Join with a plain error", errors.Join(errors.New("network error"), adapter.AuthError{}), ConnectionStateUnauthorized},
		{"rate limit takes priority over auth error in the same join", errors.Join(adapter.AuthError{}, adapter.RateLimitError{}), ConnectionStateRateLimited},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			status := newConnectionStatus(model.ProviderInstance{Name: "instance"}, sentinelTime, testCase.err)

			if status.State != testCase.wantState {
				t.Errorf("State = %v, want %v", status.State, testCase.wantState)
			}
			if status.Err != testCase.err {
				t.Errorf("Err = %v, want %v", status.Err, testCase.err)
			}
			if !status.SucceededAt.IsZero() {
				t.Errorf("SucceededAt = %v, want zero", status.SucceededAt)
			}
		})
	}
}

func TestFetch_AllHealthy(t *testing.T) {
	healthyOne := newMockPullRequestSource("one", []model.PullRequest{{Number: 1}}, nil)
	healthyTwo := newMockPullRequestSource("two", []model.PullRequest{{Number: 2}, {Number: 3}}, nil)

	client := NewWithConnections(healthyOne, healthyTwo)

	before := time.Now()
	snapshot := client.Fetch(context.Background())
	after := time.Now()

	if len(snapshot.PullRequests) != 3 {
		t.Fatalf("PullRequests = %d, want 3", len(snapshot.PullRequests))
	}
	if len(snapshot.Connections) != 2 {
		t.Fatalf("Connections = %d, want 2", len(snapshot.Connections))
	}
	if snapshot.FetchedAt.Before(before) || snapshot.FetchedAt.After(after) {
		t.Errorf("FetchedAt = %v, want between %v and %v", snapshot.FetchedAt, before, after)
	}

	for _, name := range []string{"one", "two"} {
		status, ok := connectionStatusFor(snapshot, name)
		if !ok {
			t.Fatalf("no ConnectionStatus for %q", name)
		}
		if status.State != ConnectionStateOK {
			t.Errorf("%s: State = %v, want ConnectionStateOK", name, status.State)
		}
		if status.Provider.Kind != model.ProviderGitHub {
			t.Errorf("%s: Provider.Kind = %v, want %v", name, status.Provider.Kind, model.ProviderGitHub)
		}
		if status.Err != nil {
			t.Errorf("%s: Err = %v, want nil", name, status.Err)
		}
		if status.SucceededAt.Before(before) || status.SucceededAt.After(after) {
			t.Errorf("%s: SucceededAt = %v, want between %v and %v", name, status.SucceededAt, before, after)
		}
	}
}

// TestFetch_RunsConnectionsConcurrently proves Fetch actually fans out in
// parallel rather than merely aggregating correctly regardless of order — a
// suite that only checks final state would still pass if a future change
// collapsed the goroutines into a sequential loop. Three connections at 100ms
// each would take ~300ms run serially; run concurrently they take ~100ms.
// The 250ms bound leaves a wide margin above the concurrent case and a wide
// margin below the serial case, so this should not flake under normal
// scheduling load.
func TestFetch_RunsConnectionsConcurrently(t *testing.T) {
	const delay = 100 * time.Millisecond

	sources := make([]adapter.Connection, 3)
	for index, name := range []string{"one", "two", "three"} {
		sources[index] = &mock.PullRequestSource{
			Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: name}},
			Delay:      delay,
		}
	}

	client := NewWithConnections(sources...)

	started := time.Now()
	client.Fetch(context.Background())
	elapsed := time.Since(started)

	const bound = 250 * time.Millisecond
	if elapsed >= bound {
		t.Errorf("Fetch took %v across 3 connections at %v delay each, want under %v — looks sequential, not concurrent", elapsed, delay, bound)
	}
}

// TestFetch_OneHealthyOneBroken is the issue's acceptance criterion: a fetch
// across one healthy and one broken connection returns the healthy pull
// requests plus a visible error for the broken one.
func TestFetch_OneHealthyOneBroken(t *testing.T) {
	healthy := newMockPullRequestSource("healthy", []model.PullRequest{{Number: 1}}, nil)
	brokenErr := errors.New("connection refused")
	broken := newMockPullRequestSource("broken", nil, brokenErr)

	client := NewWithConnections(healthy, broken)
	snapshot := client.Fetch(context.Background())

	if len(snapshot.PullRequests) != 1 || snapshot.PullRequests[0].Number != 1 {
		t.Fatalf("PullRequests = %+v, want just healthy's PR #1", snapshot.PullRequests)
	}

	healthyStatus, ok := connectionStatusFor(snapshot, "healthy")
	if !ok {
		t.Fatal("no ConnectionStatus for healthy")
	}
	if healthyStatus.State != ConnectionStateOK {
		t.Errorf("healthy: State = %v, want ConnectionStateOK", healthyStatus.State)
	}

	brokenStatus, ok := connectionStatusFor(snapshot, "broken")
	if !ok {
		t.Fatal("no ConnectionStatus for broken")
	}
	if brokenStatus.State != ConnectionStateOffline {
		t.Errorf("broken: State = %v, want ConnectionStateOffline", brokenStatus.State)
	}
	if !errors.Is(brokenStatus.Err, brokenErr) {
		t.Errorf("broken: Err = %v, want it to wrap %v", brokenStatus.Err, brokenErr)
	}
	if !brokenStatus.SucceededAt.IsZero() {
		t.Errorf("broken: SucceededAt = %v, want zero", brokenStatus.SucceededAt)
	}
}

func TestFetch_RateLimitedConnection(t *testing.T) {
	retryAfter := time.Now().Add(time.Hour)
	source := newMockPullRequestSource("limited", nil, adapter.RateLimitError{RetryAfter: retryAfter})

	snapshot := NewWithConnections(source).Fetch(context.Background())

	status, ok := connectionStatusFor(snapshot, "limited")
	if !ok {
		t.Fatal("no ConnectionStatus for limited")
	}
	if status.State != ConnectionStateRateLimited {
		t.Errorf("State = %v, want ConnectionStateRateLimited", status.State)
	}

	var rateLimitErr adapter.RateLimitError
	if !errors.As(status.Err, &rateLimitErr) {
		t.Fatalf("Err = %v, want it to unwrap to adapter.RateLimitError", status.Err)
	}
	if !rateLimitErr.RetryAfter.Equal(retryAfter) {
		t.Errorf("RetryAfter = %v, want %v", rateLimitErr.RetryAfter, retryAfter)
	}
}

func TestFetch_UnauthorizedConnection(t *testing.T) {
	source := newMockPullRequestSource("bad-token", nil, adapter.AuthError{})

	snapshot := NewWithConnections(source).Fetch(context.Background())

	status, ok := connectionStatusFor(snapshot, "bad-token")
	if !ok {
		t.Fatal("no ConnectionStatus for bad-token")
	}
	if status.State != ConnectionStateUnauthorized {
		t.Errorf("State = %v, want ConnectionStateUnauthorized", status.State)
	}
}

// TestFetch_RateLimitInsideJoinedError mirrors GitHub's own aggregation,
// which errors.Join()s per-repo failures: a rate limit hit on one repo must
// still classify the whole connection as RateLimited, not fall through to
// the generic Offline bucket, even alongside an unrelated plain error.
func TestFetch_RateLimitInsideJoinedError(t *testing.T) {
	joined := errors.Join(errors.New("repo-a: network error"), adapter.RateLimitError{})
	source := newMockPullRequestSource("mixed", nil, joined)

	snapshot := NewWithConnections(source).Fetch(context.Background())

	status, ok := connectionStatusFor(snapshot, "mixed")
	if !ok {
		t.Fatal("no ConnectionStatus for mixed")
	}
	if status.State != ConnectionStateRateLimited {
		t.Errorf("State = %v, want ConnectionStateRateLimited", status.State)
	}
}

func TestFetch_NoPullRequestSources(t *testing.T) {
	identityOnly := &identityOnlyConnection{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "identity-only"}},
	}

	snapshot := NewWithConnections(identityOnly).Fetch(context.Background())

	if len(snapshot.PullRequests) != 0 {
		t.Errorf("PullRequests = %d, want 0", len(snapshot.PullRequests))
	}
	if len(snapshot.Connections) != 0 {
		t.Errorf("Connections = %d, want 0: identity-only connections do not serve pull requests", len(snapshot.Connections))
	}
}

// TestFetch_NoConnectionsAtAll is distinct from TestFetch_NoPullRequestSources:
// this client holds no connections whatsoever, rather than one connection
// that simply doesn't serve pull requests. Both take the same code path
// today, but locking in the zero-connections case explicitly keeps that an
// asserted fact rather than an untested coincidence.
func TestFetch_NoConnectionsAtAll(t *testing.T) {
	snapshot := NewWithConnections().Fetch(context.Background())

	if len(snapshot.PullRequests) != 0 {
		t.Errorf("PullRequests = %d, want 0", len(snapshot.PullRequests))
	}
	if len(snapshot.Connections) != 0 {
		t.Errorf("Connections = %d, want 0: no connections were passed to NewWithConnections", len(snapshot.Connections))
	}
}
