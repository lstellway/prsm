package prsm

import (
	"context"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// defaultPollInterval is applied by PollOptions.withDefaults when Interval
// is not positive, matching config's own default refresh_interval_seconds.
const defaultPollInterval = 60 * time.Second

// PollOptions configures Client.Poll. It is caller-supplied rather than
// read from config: Client has no config reference of its own — a Client
// built via NewWithConnections has none at all — so Poll cannot reach for a
// config value itself.
type PollOptions struct {
	// Interval is how often a healthy connection is retried. A zero or
	// negative value is replaced with a 60s default rather than reaching a
	// timer with a non-positive duration.
	Interval time.Duration
	// Backoff governs retry growth and offline marking for a connection
	// that is failing. The zero value is replaced with DefaultBackoffPolicy.
	Backoff BackoffPolicy
}

// withDefaults fills in a usable Interval and BackoffPolicy for an
// otherwise zero-valued PollOptions.
func (options PollOptions) withDefaults() PollOptions {
	if options.Interval <= 0 {
		options.Interval = defaultPollInterval
	}
	if options.Backoff == (BackoffPolicy{}) {
		options.Backoff = DefaultBackoffPolicy()
	}
	return options
}

// PolledConnectionStatus reports one connection's outcome for a Poll
// engine's cycle, plus the engine's memory of it across every cycle it has
// run so far — memory Client itself deliberately does not keep; see
// ConnectionStatus's doc comment.
type PolledConnectionStatus struct {
	// ConnectionStatus.State is ConnectionStateUnknown for a connection
	// whose first fetch has not completed yet — unlike PullRequestSnapshot,
	// which always waits for every connection before it exists at all, a
	// PollSnapshot can be read (via Current, or delivered on Updates)
	// before every tracked connection has reported in even once.
	ConnectionStatus
	// ConsecutiveFailures counts how many attempts in a row have not
	// returned ConnectionStateOK. A ConnectionStateRateLimited outcome
	// neither increments nor resets this — see BackoffPolicy's doc comment.
	ConsecutiveFailures int
	// Offline is true once ConsecutiveFailures has reached the configured
	// BackoffPolicy.OfflineAfter. The connection keeps being retried on the
	// same backoff schedule; Offline only changes what is reported.
	Offline bool
	// NextAttemptAt is zero when this connection isn't currently held back
	// by backoff or a rate-limit bucket — i.e. it's on its normal polling
	// cadence. Otherwise it's the later of its own backoff schedule and its
	// bucket's hold time.
	NextAttemptAt time.Time
}

// PollSnapshot is one poll engine's current aggregate view, mirroring
// PullRequestSnapshot's shape for a single Fetch call but adding the
// backoff/offline memory only a running poll accumulates.
//
// Unlike PullRequestSnapshot, the connections here are not necessarily from
// one synchronized round: each connection is fetched on its own
// independent schedule (see Client.Poll), so AsOf is when this particular
// value was assembled — i.e. when its triggering connection's outcome was
// processed — not a synchronized fetch time for every connection. Each
// PolledConnectionStatus carries its own independent freshness via its
// embedded ConnectionStatus.SucceededAt; do not assume every connection's
// status is equally fresh just because it appears in the same PollSnapshot.
type PollSnapshot struct {
	// PullRequests is every connection's current pull requests — freshly
	// fetched or carried over from its last success — in
	// Client.pullRequestSources order.
	PullRequests []model.PullRequest
	// Connections reports one status per connection the poller is tracking,
	// in Client.pullRequestSources order.
	Connections []PolledConnectionStatus
	AsOf        time.Time
}

// Poller is a running poll loop over every connection a Client holds that
// serves pull requests, returned by Client.Poll. It is not embedded in
// Client — Client stays immutable regardless of how many times Fetch or
// Poll is called (see Client's doc comment) — and it holds no state beyond
// what its underlying engine maintains.
type Poller struct {
	engine  *pollEngine[model.PullRequest]
	updates chan PollSnapshot
}

// Poll starts a poll loop over every connection Client holds that serves
// pull requests. Each connection is fetched immediately (the startup
// burst) and then independently on its own schedule thereafter — see
// startPollEngine (poll_engine.go) for the full scheduling, backoff, and
// rate-limit-bucket-sharing behavior, which is resource-agnostic and
// identical regardless of what Client.Poll happens to fetch.
//
// The returned *Poller stops, and both Updates and Current stop being
// useful, once ctx is cancelled — there is no separate Stop method,
// matching every other ctx-taking method in this package. Two concurrent
// Poll calls against the same Client hold fully independent state, so they
// do not protect a shared vendor account from over-polling between
// themselves; only connections scheduled by the same Poll call share a
// rate-limit bucket.
func (client *Client) Poll(ctx context.Context, options PollOptions) *Poller {
	engine := startPollEngine(ctx, client.pullRequestSources, options,
		func(ctx context.Context, source adapter.PullRequestSource) ([]model.PullRequest, error) {
			return source.ListPullRequests(ctx)
		})

	poller := &Poller{
		engine:  engine,
		updates: make(chan PollSnapshot, 1),
	}
	go poller.forward(ctx)
	return poller
}

// forward translates the engine's resource-agnostic engineSnapshot into the
// pull-request-shaped PollSnapshot as each one arrives, for as long as the
// engine keeps producing them. This is the one place genericity is
// translated away, so PollSnapshot's field is named PullRequests rather
// than a generic Items shared verbatim across every future resource kind's
// snapshot type.
//
// The forward is a plain blocking send selected against ctx.Done(), with no
// second non-blocking layer of its own: if the downstream consumer is slow,
// forward simply stops draining poller.engine.updates, and the engine's own
// non-blocking latest-value send already absorbs that back-pressure by
// dropping stale buffered values — so backpressure handling isn't
// duplicated here, just inherited from the one place it's implemented.
func (poller *Poller) forward(ctx context.Context) {
	defer close(poller.updates)
	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-poller.engine.updates:
			if !ok {
				return
			}
			select {
			case poller.updates <- toPollSnapshot(snapshot):
			case <-ctx.Done():
				return
			}
		}
	}
}

func toPollSnapshot(snapshot engineSnapshot[model.PullRequest]) PollSnapshot {
	return PollSnapshot{
		PullRequests: snapshot.Items,
		Connections:  snapshot.Connections,
		AsOf:         snapshot.AsOf,
	}
}

// Updates returns the channel this poller pushes a fresh PollSnapshot onto
// every time any tracked connection's own fetch completes — not on a
// synchronized global tick. It is closed once the poller stops (ctx passed
// to Poll was cancelled).
func (poller *Poller) Updates() <-chan PollSnapshot {
	return poller.updates
}

// Current returns the poller's current known state instantly, without
// waiting on any fetch in flight — the read path for a request/response
// style caller that wants one settled answer rather than to subscribe to
// Updates. It never triggers a fresh fetch itself.
//
// Current returns the zero PollSnapshot if the poller has already stopped
// (ctx passed to Poll was cancelled) or if ctx itself is done — including
// when called with a ctx that carries no deadline of its own, after the
// poller has stopped.
func (poller *Poller) Current(ctx context.Context) PollSnapshot {
	return toPollSnapshot(poller.engine.current(ctx))
}
