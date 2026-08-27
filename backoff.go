package prsm

import (
	"errors"
	"math"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// BackoffPolicy configures how a poll engine (poll_engine.go) grows the wait
// between retries for a connection that keeps failing, and when it reports
// that connection as offline rather than merely degraded.
type BackoffPolicy struct {
	// Initial is the wait applied after the first consecutive failure.
	Initial time.Duration
	// Max caps how large the wait can grow, regardless of how many
	// consecutive failures have piled up.
	Max time.Duration
	// Multiplier grows the wait by this factor for each additional
	// consecutive failure, until Max is reached.
	Multiplier float64
	// OfflineAfter is the number of consecutive failures at which a
	// connection is reported offline (PolledConnectionStatus.Offline). It
	// keeps being retried on the same backoff schedule regardless;
	// OfflineAfter only changes what is reported, not whether retries
	// continue.
	OfflineAfter int
}

// DefaultBackoffPolicy is applied when a PollOptions is used without an
// explicit BackoffPolicy.
func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{
		Initial:      30 * time.Second,
		Max:          30 * time.Minute,
		Multiplier:   2,
		OfflineAfter: 3,
	}
}

// wait returns how long to back off after failureCount consecutive
// failures: Initial * Multiplier^(failureCount-1), capped at Max.
func (policy BackoffPolicy) wait(failureCount int) time.Duration {
	growth := math.Pow(policy.Multiplier, float64(failureCount-1))
	waitDuration := float64(policy.Initial) * growth
	if capped := float64(policy.Max); waitDuration > capped {
		waitDuration = capped
	}
	return time.Duration(waitDuration)
}

// defaultRateLimitBackoff is applied when a RateLimitError does not carry a
// RetryAfter — see adapter.RateLimitError's doc comment.
const defaultRateLimitBackoff = 60 * time.Second

// rateLimitRetryAfter extracts the RetryAfter an adapter.RateLimitError
// reported, defaulting to now plus defaultRateLimitBackoff when it's zero
// (the provider gave no reset time) or err does not wrap a RateLimitError at
// all.
func rateLimitRetryAfter(err error, now time.Time) time.Time {
	var rateLimitErr adapter.RateLimitError
	if errors.As(err, &rateLimitErr) && !rateLimitErr.RetryAfter.IsZero() {
		return rateLimitErr.RetryAfter
	}
	return now.Add(defaultRateLimitBackoff)
}

// rateLimitBucketKey identifies the vendor account a connection's rate
// limit is actually enforced against, independent of how many configured
// connections reach it. Two connections with different Names but the same
// Kind, Host, and Account share one vendor-side rate limit; scheduling them
// independently would mean one connection's 429 does not hold the other
// back, and the pair would trip the same limit twice as fast as a single
// connection would.
//
// When Account is still empty — adapter/github's Instance().Account, for
// example, is only populated once ResolveIdentity has run — keying on
// Kind+Host+Account alone would collapse every not-yet-resolved connection
// on the same host into one bucket, even if they are genuinely different
// accounts. Falling back to Kind+Host+Name instead means "don't share a
// bucket unless the account is actually known to match." The two cases are
// tagged so a Name-fallback key can never collide with an Account key.
func rateLimitBucketKey(instance model.ProviderInstance) string {
	if instance.Account != "" {
		return "account\x00" + string(instance.Kind) + "\x00" + instance.Host + "\x00" + instance.Account
	}
	return "name\x00" + string(instance.Kind) + "\x00" + instance.Host + "\x00" + instance.Name
}

// rateLimitBucket is a poll engine's private memory for one vendor account,
// shared by any number of configured connections that map to the same
// rateLimitBucketKey.
type rateLimitBucket struct {
	until time.Time
}

// due reports whether this bucket currently allows a fetch at now.
func (bucket *rateLimitBucket) due(now time.Time) bool {
	return bucket.until.IsZero() || !bucket.until.After(now)
}

// hold extends the bucket's hold time forward to retryAfter. It never
// shortens an existing hold: if two connections sharing this bucket report
// different RetryAfter values, the later one wins — waking up too early
// just re-trips the same limit.
func (bucket *rateLimitBucket) hold(retryAfter time.Time) {
	if retryAfter.After(bucket.until) {
		bucket.until = retryAfter
	}
}

// connectionSchedule is a poll engine's private memory for one connection,
// carried across every outcome it reports. It is touched only by the poll
// engine's owner goroutine (poll_engine.go) — never shared across
// goroutines directly — so it needs no locking of its own.
//
// Item is the fetched payload type — model.PullRequest for Client.Poll
// today, a future resource kind's own model type later — the only reason
// this type is generic; everything else about scheduling a connection's
// retries is resource-agnostic.
type connectionSchedule[Item any] struct {
	consecutiveFailures int
	nextAttemptAt       time.Time
	lastItems           []Item
	lastStatus          ConnectionStatus
}

// recordSuccess resets the failure streak and clears any scheduled backoff —
// the connection is eligible again immediately — and replaces the
// connection's contribution to the aggregate view with this call's items.
func (schedule *connectionSchedule[Item]) recordSuccess(items []Item, status ConnectionStatus) {
	schedule.consecutiveFailures = 0
	schedule.nextAttemptAt = time.Time{}
	schedule.lastItems = items
	schedule.lastStatus = status
}

// recordFailure grows the failure streak and computes the next backoff wait
// via policy. It deliberately does not touch lastItems: a transient failure
// must not make a connection's items flicker out of the aggregate view —
// Fetch already fully replaces, rather than diffs, a connection's
// contribution on every call, so carrying the last-known items forward
// through a failure is consistent with existing behavior, not new.
func (schedule *connectionSchedule[Item]) recordFailure(now time.Time, policy BackoffPolicy, status ConnectionStatus) {
	schedule.consecutiveFailures++
	schedule.nextAttemptAt = now.Add(policy.wait(schedule.consecutiveFailures))
	schedule.lastStatus = status
}

// recordRateLimited updates only lastStatus. A rate limit is an expected
// vendor throttle, not evidence the connection is unhealthy, so it does not
// grow the failure streak and does not touch nextAttemptAt — scheduling for
// a rate-limited connection is driven by its shared rateLimitBucket instead
// (poll_engine.go) — or lastItems, for the same reason as recordFailure.
func (schedule *connectionSchedule[Item]) recordRateLimited(status ConnectionStatus) {
	schedule.lastStatus = status
}

// offline reports whether this connection's failure streak has crossed
// policy's threshold. It keeps being retried on the same backoff schedule
// regardless; this only changes what is reported.
func (schedule *connectionSchedule[Item]) offline(policy BackoffPolicy) bool {
	return schedule.consecutiveFailures >= policy.OfflineAfter
}
