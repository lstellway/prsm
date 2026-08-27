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

// stubConnection is a minimal adapter.Connection used only to drive
// runPollOwner directly in these tests, bypassing real workers and real
// time entirely. account is settable so bucket-sharing tests can put two
// stubConnections on the same vendor account.
type stubConnection struct {
	name    string
	account string
}

func (connection stubConnection) Instance() model.ProviderInstance {
	return model.ProviderInstance{Name: connection.name, Kind: model.ProviderGitHub, Host: "github.com", Account: connection.account}
}

// stubItem is a deliberately non-model type, used once below to prove
// startPollEngine is genuinely generic — not secretly relying on anything
// specific to model.PullRequest.
type stubItem struct {
	value string
}

// startTestEngine runs runPollOwner directly against a synthetic reports
// channel, with no worker goroutines and no real timers — the owner's
// backoff/bucket decision logic is exercised with hand-built outcomes only.
func startTestEngine[Item any](ctx context.Context, sources []stubConnection, options PollOptions) (*pollEngine[Item], chan pollReport[Item]) {
	engine := &pollEngine[Item]{
		updates: make(chan engineSnapshot[Item], 1),
		queries: make(chan chan engineSnapshot[Item]),
		done:    make(chan struct{}),
	}
	reports := make(chan pollReport[Item])
	go runPollOwner(ctx, sources, options.withDefaults(), reports, engine)
	return engine, reports
}

// sendReport sends one outcome to the owner and returns the next-wake time
// it replies with, failing the test if the owner doesn't reply promptly.
func sendReport[Item any](t *testing.T, reports chan<- pollReport[Item], instance model.ProviderInstance, items []Item, err error, observedAt time.Time) time.Time {
	t.Helper()
	replyChan := make(chan time.Time, 1)
	reports <- pollReport[Item]{
		outcome:  pollOutcome[Item]{instance: instance, items: items, err: err, observedAt: observedAt},
		nextWake: replyChan,
	}
	select {
	case wake := <-replyChan:
		return wake
	case <-time.After(time.Second):
		t.Fatal("owner did not reply with a next-wake time within 1s")
		return time.Time{}
	}
}

func polledStatusFor[Item any](t *testing.T, snapshot engineSnapshot[Item], name string) PolledConnectionStatus {
	t.Helper()
	for _, status := range snapshot.Connections {
		if status.Provider.Name == name {
			return status
		}
	}
	t.Fatalf("no PolledConnectionStatus for %q", name)
	return PolledConnectionStatus{}
}

func TestPollEngine_BackoffGrowsAndMarksOffline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flaky := stubConnection{name: "flaky"}
	options := PollOptions{
		Interval: time.Hour,
		Backoff:  BackoffPolicy{Initial: 10 * time.Second, Max: time.Minute, Multiplier: 2, OfflineAfter: 3},
	}
	engine, reports := startTestEngine[string](ctx, []stubConnection{flaky}, options)

	now := time.Now()
	boom := errors.New("boom")

	wake := sendReport(t, reports, flaky.Instance(), nil, boom, now)
	if want := now.Add(10 * time.Second); !wake.Equal(want) {
		t.Errorf("wake after 1st failure = %v, want %v", wake, want)
	}
	status := polledStatusFor(t, engine.current(ctx), "flaky")
	if status.ConsecutiveFailures != 1 || status.Offline {
		t.Errorf("after 1 failure: ConsecutiveFailures=%d Offline=%v, want 1 false", status.ConsecutiveFailures, status.Offline)
	}

	wake = sendReport(t, reports, flaky.Instance(), nil, boom, now)
	if want := now.Add(20 * time.Second); !wake.Equal(want) {
		t.Errorf("wake after 2nd failure = %v, want %v", wake, want)
	}
	status = polledStatusFor(t, engine.current(ctx), "flaky")
	if status.ConsecutiveFailures != 2 || status.Offline {
		t.Errorf("after 2 failures: ConsecutiveFailures=%d Offline=%v, want 2 false", status.ConsecutiveFailures, status.Offline)
	}

	wake = sendReport(t, reports, flaky.Instance(), nil, boom, now)
	if want := now.Add(40 * time.Second); !wake.Equal(want) {
		t.Errorf("wake after 3rd failure = %v, want %v", wake, want)
	}
	status = polledStatusFor(t, engine.current(ctx), "flaky")
	if status.ConsecutiveFailures != 3 || !status.Offline {
		t.Errorf("after 3 failures: ConsecutiveFailures=%d Offline=%v, want 3 true (OfflineAfter=3)", status.ConsecutiveFailures, status.Offline)
	}
}

func TestPollEngine_SuccessResetsFailureStreak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flaky := stubConnection{name: "flaky"}
	options := PollOptions{
		Interval: time.Minute,
		Backoff:  BackoffPolicy{Initial: 10 * time.Second, Max: time.Hour, Multiplier: 2, OfflineAfter: 2},
	}
	engine, reports := startTestEngine[string](ctx, []stubConnection{flaky}, options)

	now := time.Now()
	sendReport(t, reports, flaky.Instance(), nil, errors.New("boom"), now)
	sendReport(t, reports, flaky.Instance(), nil, errors.New("boom"), now)
	if status := polledStatusFor(t, engine.current(ctx), "flaky"); !status.Offline {
		t.Fatal("expected connection to be Offline after 2 failures (OfflineAfter=2) before testing recovery")
	}

	wake := sendReport(t, reports, flaky.Instance(), []string{"recovered"}, nil, now)
	if want := now.Add(time.Minute); !wake.Equal(want) {
		t.Errorf("wake after success = %v, want now+Interval = %v", wake, want)
	}

	status := polledStatusFor(t, engine.current(ctx), "flaky")
	if status.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures after success = %d, want 0", status.ConsecutiveFailures)
	}
	if status.Offline {
		t.Error("Offline after success = true, want false")
	}
	if !status.NextAttemptAt.IsZero() {
		t.Errorf("NextAttemptAt after success = %v, want zero (not held back)", status.NextAttemptAt)
	}
}

func TestPollEngine_RateLimitSharesBucketAcrossConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connectionA := stubConnection{name: "conn-a", account: "alice"}
	connectionB := stubConnection{name: "conn-b", account: "alice"}
	options := PollOptions{Interval: time.Minute, Backoff: DefaultBackoffPolicy()}
	engine, reports := startTestEngine[string](ctx, []stubConnection{connectionA, connectionB}, options)

	now := time.Now()
	retryAfter := now.Add(time.Hour)
	wake := sendReport(t, reports, connectionA.Instance(), nil, adapter.RateLimitError{RetryAfter: retryAfter}, now)
	if !wake.Equal(retryAfter) {
		t.Errorf("wake for the rate-limited connection = %v, want %v", wake, retryAfter)
	}

	snapshot := engine.current(ctx)

	statusA := polledStatusFor(t, snapshot, "conn-a")
	if statusA.ConsecutiveFailures != 0 {
		t.Errorf("conn-a ConsecutiveFailures = %d, want 0 — a rate limit is not a failure", statusA.ConsecutiveFailures)
	}
	if !statusA.NextAttemptAt.Equal(retryAfter) {
		t.Errorf("conn-a NextAttemptAt = %v, want %v", statusA.NextAttemptAt, retryAfter)
	}

	statusB := polledStatusFor(t, snapshot, "conn-b")
	if statusB.ConsecutiveFailures != 0 {
		t.Errorf("conn-b ConsecutiveFailures = %d, want 0 — conn-b never itself failed", statusB.ConsecutiveFailures)
	}
	if !statusB.NextAttemptAt.Equal(retryAfter) {
		t.Errorf("conn-b NextAttemptAt = %v, want %v — it shares conn-a's account and must be held back too", statusB.NextAttemptAt, retryAfter)
	}
}

func TestPollEngine_FailureDoesNotEraseLastItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connection := stubConnection{name: "flaky"}
	options := PollOptions{Interval: time.Minute, Backoff: DefaultBackoffPolicy()}
	engine, reports := startTestEngine[string](ctx, []stubConnection{connection}, options)

	now := time.Now()
	sendReport(t, reports, connection.Instance(), []string{"a", "b"}, nil, now)
	snapshot := engine.current(ctx)
	if got := snapshot.Items; len(got) != 2 {
		t.Fatalf("Items after success = %v, want [a b]", got)
	}

	sendReport(t, reports, connection.Instance(), nil, errors.New("boom"), now)
	snapshot = engine.current(ctx)
	if got := snapshot.Items; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Items after a subsequent failure = %v, want unchanged [a b]", got)
	}
	status := polledStatusFor(t, snapshot, "flaky")
	if status.State != ConnectionStateOffline {
		t.Errorf("State after failure = %v, want ConnectionStateOffline", status.State)
	}
}

// TestStartPollEngine_GenericOverNonPullRequestItem locks in that
// startPollEngine compiles and behaves identically for a connection type
// and item type that have nothing to do with model.PullRequest — the whole
// point of splitting this engine out generically rather than writing it
// concretely for pull requests.
func TestStartPollEngine_GenericOverNonPullRequestItem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := stubConnection{name: "generic-source"}
	engine := startPollEngine(ctx, []stubConnection{source}, PollOptions{Interval: time.Hour},
		func(_ context.Context, _ stubConnection) ([]stubItem, error) {
			return []stubItem{{value: "hello"}}, nil
		})

	select {
	case snapshot := <-engine.updates:
		if len(snapshot.Items) != 1 || snapshot.Items[0].value != "hello" {
			t.Errorf("Items = %+v, want one stubItem{value: hello}", snapshot.Items)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no update received within 2s of the startup burst")
	}
}

func TestStartPollEngine_WorkerBacksOffBetweenFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const backoffDuration = 200 * time.Millisecond
	source := &mock.PullRequestSource{
		Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "flaky", Kind: model.ProviderGitHub}},
		PullRequestsErr: errors.New("boom"),
	}

	options := PollOptions{
		Interval: time.Hour, // irrelevant: this connection fails every attempt
		Backoff:  BackoffPolicy{Initial: backoffDuration, Max: time.Minute, Multiplier: 2, OfflineAfter: 100},
	}

	startPollEngine(ctx, []adapter.PullRequestSource{source}, options,
		func(ctx context.Context, source adapter.PullRequestSource) ([]model.PullRequest, error) {
			return source.ListPullRequests(ctx)
		})

	time.Sleep(50 * time.Millisecond)
	if got := source.CallCount.Load(); got != 1 {
		t.Fatalf("CallCount after the startup burst = %d, want 1", got)
	}

	time.Sleep(100 * time.Millisecond) // cumulative 150ms, still short of backoffDuration
	if got := source.CallCount.Load(); got != 1 {
		t.Errorf("CallCount = %d before backoff elapsed, want still 1", got)
	}

	time.Sleep(300 * time.Millisecond) // cumulative 450ms, comfortably past backoffDuration
	if got := source.CallCount.Load(); got < 2 {
		t.Errorf("CallCount = %d after backoff elapsed, want >= 2", got)
	}
}

func TestStartPollEngine_ShutsDownPromptlyOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	source := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "healthy", Kind: model.ProviderGitHub}},
		PullRequests: []model.PullRequest{{Number: 1}},
	}

	engine := startPollEngine(ctx, []adapter.PullRequestSource{source}, PollOptions{Interval: time.Hour},
		func(ctx context.Context, source adapter.PullRequestSource) ([]model.PullRequest, error) {
			return source.ListPullRequests(ctx)
		})

	// Let the startup burst land and the worker settle into its (hour-long)
	// sleep before cancelling, so this exercises mid-sleep cancellation
	// rather than racing the first fetch.
	<-engine.updates
	cancel()

	select {
	case <-engine.done:
	case <-time.After(time.Second):
		t.Fatal("engine.done was not closed within 1s of ctx cancellation")
	}
}
