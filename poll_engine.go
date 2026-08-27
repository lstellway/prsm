package prsm

import (
	"context"
	"fmt"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// pollOutcome is what one poll engine worker reports to its owner after one
// attempt to fetch from its connection.
type pollOutcome[Item any] struct {
	instance   model.ProviderInstance
	items      []Item
	err        error
	observedAt time.Time
}

// pollReport pairs an outcome with the channel the owner replies on with
// this worker's next wake time. The owner alone decides scheduling, since
// it alone holds the backoff and rate-limit-bucket state a decision can
// depend on — see startPollEngine's doc comment.
type pollReport[Item any] struct {
	outcome  pollOutcome[Item]
	nextWake chan<- time.Time
}

// engineSnapshot is the resource-agnostic shape a poll engine maintains:
// every connection's current items — freshly fetched or carried over from
// its last success — plus its polled status. Client.Poll (poll.go) maps
// this into the pull-request-shaped, exported PollSnapshot; a future
// resource kind would map the same engine into its own exported type.
//
// AsOf is when this particular value was assembled — i.e. when its
// triggering connection's outcome was processed — not a synchronized fetch
// round across every connection. Each PolledConnectionStatus carries its
// own independent freshness via its embedded ConnectionStatus.SucceededAt.
type engineSnapshot[Item any] struct {
	Items       []Item
	Connections []PolledConnectionStatus
	AsOf        time.Time
}

// pollEngine is the resource-agnostic owner/worker engine behind
// Client.Poll: one owner goroutine holds all scheduling state, and any
// number of independent worker goroutines each fetch from one connection on
// their own schedule and report outcomes to the owner. See startPollEngine.
type pollEngine[Item any] struct {
	updates chan engineSnapshot[Item]
	queries chan chan engineSnapshot[Item]
	done    chan struct{}
}

// current answers with the engine's current known state, without waiting on
// anything in flight. It selects on the engine's own done channel, in
// addition to ctx, on both the send and the receive: without that, a caller
// whose own ctx has no deadline would hang forever once the engine's owner
// has already exited (its ctx was cancelled) and nothing is left to answer
// the query. Returns the zero engineSnapshot if the engine has stopped or
// ctx is done.
func (engine *pollEngine[Item]) current(ctx context.Context) engineSnapshot[Item] {
	reply := make(chan engineSnapshot[Item], 1)
	select {
	case engine.queries <- reply:
	case <-ctx.Done():
		return engineSnapshot[Item]{}
	case <-engine.done:
		return engineSnapshot[Item]{}
	}

	select {
	case snapshot := <-reply:
		return snapshot
	case <-ctx.Done():
		return engineSnapshot[Item]{}
	case <-engine.done:
		return engineSnapshot[Item]{}
	}
}

// startPollEngine spawns one worker goroutine per source plus one owner
// goroutine, and returns immediately. call is the one resource-specific
// thing passed in — source.ListPullRequests for pull requests, a future
// source.ListActions for CI runs, and so on; neither goroutine below cares
// what Item or S actually are, only that call fetches a list of Item from
// one S.
//
// Every source is fetched immediately on start (the startup burst), then
// again on its own independent schedule thereafter: a connection that keeps
// failing backs off according to options.Backoff instead of being retried
// every cycle, and a connection sharing a rate-limit bucket with another
// (see rateLimitBucketKey) waits until that bucket clears, independent of
// any other connection's schedule. One slow or hanging connection never
// delays another connection's fresh results from reaching engine.updates.
func startPollEngine[S adapter.Connection, Item any](
	ctx context.Context,
	sources []S,
	options PollOptions,
	call func(context.Context, S) ([]Item, error),
) *pollEngine[Item] {
	options = options.withDefaults()

	engine := &pollEngine[Item]{
		updates: make(chan engineSnapshot[Item], 1),
		queries: make(chan chan engineSnapshot[Item]),
		done:    make(chan struct{}),
	}

	reports := make(chan pollReport[Item])
	for _, source := range sources {
		go runPollWorker(ctx, source, call, reports)
	}

	go runPollOwner(ctx, sources, options, reports, engine)

	return engine
}

// runPollWorker owns exactly one connection: sleep until due, fetch, report
// the outcome to the owner, receive back the next wake time, repeat — until
// ctx is done. It re-reads source.Instance() fresh on every attempt rather
// than caching it once, since fields such as Account can resolve after
// startup (see rateLimitBucketKey's doc comment).
func runPollWorker[S adapter.Connection, Item any](
	ctx context.Context,
	source S,
	call func(context.Context, S) ([]Item, error),
	reports chan<- pollReport[Item],
) {
	wake := time.Now() // eligible immediately: the startup burst

	for {
		timer := time.NewTimer(time.Until(wake))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		items, err := fetchRecoveringPanic(ctx, source, call)

		replyChan := make(chan time.Time, 1)
		report := pollReport[Item]{
			outcome: pollOutcome[Item]{
				instance:   source.Instance(),
				items:      items,
				err:        err,
				observedAt: time.Now(),
			},
			nextWake: replyChan,
		}

		select {
		case reports <- report:
		case <-ctx.Done():
			return
		}

		select {
		case wake = <-replyChan:
		case <-ctx.Done():
			return
		}
	}
}

// fetchRecoveringPanic calls call and recovers a panic into an error rather
// than letting it escape, mirroring fanOut's per-goroutine recover
// (fanout.go). Recovering here, inside the worker's loop, rather than once
// around the whole loop, means one panic is reported as a single failed
// outcome and this connection keeps being retried on its normal schedule,
// instead of silently stopping forever.
func fetchRecoveringPanic[S adapter.Connection, Item any](
	ctx context.Context,
	source S,
	call func(context.Context, S) ([]Item, error),
) (items []Item, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return call(ctx, source)
}

// runPollOwner is the only code that ever reads or writes
// connectionSchedule/rateLimitBucket state for one engine. It answers
// synchronous queries and processes worker reports strictly one at a time
// from a single select loop, so no locking is needed anywhere in this file:
// deciding whether a connection is eligible can depend on another
// connection's most recent outcome (a shared rateLimitBucket), which is
// exactly the kind of multi-key decision that a per-entry lock/unlock
// pattern gets racy.
func runPollOwner[S adapter.Connection, Item any](
	ctx context.Context,
	sources []S,
	options PollOptions,
	reports <-chan pollReport[Item],
	engine *pollEngine[Item],
) {
	defer close(engine.updates)
	defer close(engine.done)

	schedules := make(map[string]*connectionSchedule[Item], len(sources))
	for _, source := range sources {
		schedules[source.Instance().Name] = &connectionSchedule[Item]{}
	}
	buckets := make(map[string]*rateLimitBucket)

	current := engineSnapshot[Item]{}

	for {
		select {
		case <-ctx.Done():
			return

		case reply := <-engine.queries:
			reply <- current

		case report := <-reports:
			instance := report.outcome.instance
			schedule := schedules[instance.Name]
			status := newConnectionStatus(instance, report.outcome.observedAt, report.outcome.err)

			switch status.State {
			case ConnectionStateOK:
				schedule.recordSuccess(report.outcome.items, status)
			case ConnectionStateRateLimited:
				bucketKey := rateLimitBucketKey(instance)
				bucket := buckets[bucketKey]
				if bucket == nil {
					bucket = &rateLimitBucket{}
					buckets[bucketKey] = bucket
				}
				bucket.hold(rateLimitRetryAfter(status.Err, report.outcome.observedAt))
				schedule.recordRateLimited(status)
			default:
				schedule.recordFailure(report.outcome.observedAt, options.Backoff, status)
			}

			report.nextWake <- nextWakeFor(instance, schedule, buckets, report.outcome.observedAt, options)

			current = buildEngineSnapshot(sources, schedules, buckets, options.Backoff, report.outcome.observedAt)

			select {
			case engine.updates <- current:
			default:
				select {
				case <-engine.updates:
				default:
				}
				engine.updates <- current
			}
		}
	}
}

// nextWakeFor computes when a connection's worker should wake to attempt
// its next fetch: the later of its own backoff schedule and its shared
// rate-limit bucket's hold time, or observedAt plus the configured Interval
// when neither is holding it back.
func nextWakeFor[Item any](
	instance model.ProviderInstance,
	schedule *connectionSchedule[Item],
	buckets map[string]*rateLimitBucket,
	observedAt time.Time,
	options PollOptions,
) time.Time {
	wake := schedule.nextAttemptAt
	if wake.IsZero() {
		wake = observedAt.Add(options.Interval)
	}
	if bucket := buckets[rateLimitBucketKey(instance)]; bucket != nil && bucket.until.After(wake) {
		wake = bucket.until
	}
	return wake
}

// buildEngineSnapshot rebuilds the full aggregate view from every source's
// current schedule state, in sources order — the same order Fetch already
// documents and preserves for PullRequestSnapshot.
func buildEngineSnapshot[S adapter.Connection, Item any](
	sources []S,
	schedules map[string]*connectionSchedule[Item],
	buckets map[string]*rateLimitBucket,
	backoffPolicy BackoffPolicy,
	asOf time.Time,
) engineSnapshot[Item] {
	var allItems []Item
	statuses := make([]PolledConnectionStatus, 0, len(sources))

	for _, source := range sources {
		instance := source.Instance()
		schedule := schedules[instance.Name]
		allItems = append(allItems, schedule.lastItems...)

		nextAttemptAt := schedule.nextAttemptAt
		if bucket := buckets[rateLimitBucketKey(instance)]; bucket != nil && bucket.until.After(nextAttemptAt) {
			nextAttemptAt = bucket.until
		}

		// A connection that has not reported an outcome yet (its first
		// fetch is still in flight, or hasn't started) has a zero-value
		// lastStatus, including a zero-value Provider — set it from the
		// live instance explicitly, so every connection is always
		// correctly identified in the aggregate view, not just the ones
		// that have reported at least once.
		connectionStatus := schedule.lastStatus
		connectionStatus.Provider = instance

		statuses = append(statuses, PolledConnectionStatus{
			ConnectionStatus:    connectionStatus,
			ConsecutiveFailures: schedule.consecutiveFailures,
			Offline:             schedule.offline(backoffPolicy),
			NextAttemptAt:       nextAttemptAt,
		})
	}

	return engineSnapshot[Item]{Items: allItems, Connections: statuses, AsOf: asOf}
}
