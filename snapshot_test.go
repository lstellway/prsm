package prsm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
)

func connectionStatusFor(snapshot Snapshot, name string) (ConnectionStatus, bool) {
	for _, status := range snapshot.Connections {
		if status.Name == name {
			return status, true
		}
	}
	return ConnectionStatus{}, false
}

func TestFetch_AllHealthy(t *testing.T) {
	healthyOne := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "one", Kind: model.ProviderGitHub}},
		PullRequests: []model.PullRequest{{Number: 1}},
	}
	healthyTwo := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "two", Kind: model.ProviderGitHub}},
		PullRequests: []model.PullRequest{{Number: 2}, {Number: 3}},
	}

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
		if status.Err != nil {
			t.Errorf("%s: Err = %v, want nil", name, status.Err)
		}
		if status.SucceededAt.Before(before) || status.SucceededAt.After(after) {
			t.Errorf("%s: SucceededAt = %v, want between %v and %v", name, status.SucceededAt, before, after)
		}
	}
}

// TestFetch_OneHealthyOneBroken is the issue's acceptance criterion: a fetch
// across one healthy and one broken connection returns the healthy pull
// requests plus a visible error for the broken one.
func TestFetch_OneHealthyOneBroken(t *testing.T) {
	healthy := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "healthy", Kind: model.ProviderGitHub}},
		PullRequests: []model.PullRequest{{Number: 1}},
	}
	brokenErr := errors.New("connection refused")
	broken := &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "broken", Kind: model.ProviderGitHub}},
		PullRequestsErr: brokenErr,
	}

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
	source := &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "limited"}},
		PullRequestsErr: adapter.RateLimitError{RetryAfter: retryAfter},
	}

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
	source := &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "bad-token"}},
		PullRequestsErr: adapter.AuthError{},
	}

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
	source := &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "mixed"}},
		PullRequestsErr: joined,
	}

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
