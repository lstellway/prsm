package prsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// ConnectionState is the closed set of outcomes one Fetch call can record for
// a single connection. It answers only for the call that just happened —
// there is no persisted history here. A poll loop that wants to know how
// long a connection has been down, or when to back off, holds that history
// itself across repeated calls to Fetch; Client stays immutable regardless
// of how many times Fetch is called. See Client's doc comment.
type ConnectionState int

const (
	// ConnectionStateUnknown is the zero value: no outcome has been recorded.
	// Like PRStateUnknown and the other model sentinels, a ConnectionStatus
	// in a PullRequestSnapshot should never hold it; it exists so a partial literal
	// cannot pass itself off as healthy.
	ConnectionStateUnknown ConnectionState = iota
	// ConnectionStateOK means ListPullRequests returned with no error.
	ConnectionStateOK
	// ConnectionStateOffline covers every failure that is not one of the
	// more specific states below — network errors, timeouts, 5xx responses,
	// and anything else ListPullRequests returned as a plain error.
	ConnectionStateOffline
	// ConnectionStateRateLimited means the connection returned an
	// adapter.RateLimitError.
	ConnectionStateRateLimited
	// ConnectionStateUnauthorized means the connection returned an
	// adapter.AuthError — a bad or expired credential, distinct from a
	// vendor being unreachable because the fix is different (rotate the
	// credential, not wait and retry).
	ConnectionStateUnauthorized
)

// IsKnown reports whether an outcome has been recorded. False for the zero value.
func (connectionState ConnectionState) IsKnown() bool {
	return connectionState != ConnectionStateUnknown
}

func (connectionState ConnectionState) String() string {
	switch connectionState {
	case ConnectionStateUnknown:
		return "ConnectionStateUnknown"
	case ConnectionStateOK:
		return "ConnectionStateOK"
	case ConnectionStateOffline:
		return "ConnectionStateOffline"
	case ConnectionStateRateLimited:
		return "ConnectionStateRateLimited"
	case ConnectionStateUnauthorized:
		return "ConnectionStateUnauthorized"
	default:
		return fmt.Sprintf("ConnectionState(%d)", int(connectionState))
	}
}

// Label returns the short, stable, lowercase name a consumer-facing surface
// (CLI output, JSON) should print for this state — unlike String, which is
// meant for Go-facing debug/error text. Every known ConnectionState has a
// Label; only the zero value falls through to "unknown", since a
// ConnectionStatus should never carry ConnectionStateUnknown in the first
// place (see its doc comment).
func (connectionState ConnectionState) Label() string {
	switch connectionState {
	case ConnectionStateOK:
		return "ok"
	case ConnectionStateOffline:
		return "offline"
	case ConnectionStateRateLimited:
		return "rate_limited"
	case ConnectionStateUnauthorized:
		return "unauthorized"
	default:
		return "unknown"
	}
}

// ConnectionStatus reports one connection's outcome for the Fetch call that
// produced the enclosing PullRequestSnapshot.
type ConnectionStatus struct {
	Provider model.ProviderInstance
	State    ConnectionState
	// SucceededAt is when this connection's own ListPullRequests call
	// returned successfully — not when Fetch was invoked, and not a memory
	// of some earlier, different call; it is the zero value on failure. A
	// consumer that wants "how long has this been down" tracks that itself
	// across successive PullRequestSnapshots.
	SucceededAt time.Time
	// Err is nil when State is ConnectionStateOK, and the cause otherwise.
	// It is the same error ListPullRequests returned, so errors.As still
	// reaches adapter.RateLimitError / adapter.AuthError through it.
	Err error
}

// PullRequestSnapshot is one point-in-time result of fetching pull requests across
// every connection a Client holds. It carries no memory of any earlier
// PullRequestSnapshot — see ConnectionStatus.SucceededAt.
type PullRequestSnapshot struct {
	// PullRequests is every pull request returned by every connection that
	// served pull requests, in Client.pullRequestSources order. A
	// connection whose ListPullRequests call partially failed still
	// contributes whatever it returned — see Connections for that
	// connection's degraded status.
	PullRequests []model.PullRequest
	// Connections reports one status per connection Fetch attempted, in
	// Client.pullRequestSources order.
	Connections []ConnectionStatus
	// FetchedAt is when this PullRequestSnapshot's Fetch call was made.
	FetchedAt time.Time
}

// Fetch fans out ListPullRequests across every connection that serves pull
// requests and aggregates the results into a PullRequestSnapshot. One connection
// failing does not fail the others: a connection whose call errors still
// contributes whatever pull requests it did return, with its degraded
// status recorded on Connections.
//
// Fetch never returns an error itself — even a PullRequestSnapshot where every
// connection failed is a valid result, not a failure to produce one. This
// mirrors New: partial (or total) per-connection failure is reported
// through the result's structure, never through a top-level error that
// would invite a caller to discard an otherwise-usable PullRequestSnapshot.
func (client *Client) Fetch(ctx context.Context) PullRequestSnapshot {
	fetchedAt := time.Now()

	pullRequestsByConnection := make([][]model.PullRequest, len(client.pullRequestSources))
	connections := make([]ConnectionStatus, len(client.pullRequestSources))

	var waitGroup sync.WaitGroup
	for index, pullRequestSource := range client.pullRequestSources {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					connections[index] = newConnectionStatus(
						pullRequestSource.Instance(), fetchedAt, fmt.Errorf("panic: %v", recovered))
				}
			}()

			pullRequests, err := pullRequestSource.ListPullRequests(ctx)
			succeededAt := time.Now()
			pullRequestsByConnection[index] = pullRequests
			connections[index] = newConnectionStatus(pullRequestSource.Instance(), succeededAt, err)
		}()
	}
	waitGroup.Wait()

	var allPullRequests []model.PullRequest
	for _, pullRequests := range pullRequestsByConnection {
		allPullRequests = append(allPullRequests, pullRequests...)
	}

	return PullRequestSnapshot{
		PullRequests: allPullRequests,
		Connections:  connections,
		FetchedAt:    fetchedAt,
	}
}

// newConnectionStatus classifies err into a ConnectionState via
// classifyConnectionState and records succeededAt only on success.
func newConnectionStatus(instance model.ProviderInstance, succeededAt time.Time, err error) ConnectionStatus {
	status := ConnectionStatus{Provider: instance, Err: err, State: classifyConnectionState(err)}
	if err == nil {
		status.SucceededAt = succeededAt
	}
	return status
}

// classifyConnectionState maps err into the ConnectionState it represents.
// Shared by newConnectionStatus (Fetch) and newIdentityStatus
// (Client.ResolveIdentities, in identity.go): both wrap a call to a vendor
// API and fail the same three ways, even though the two calls run on
// different schedules and are never merged into one result type.
//
// A RateLimitError or AuthError anywhere in err — including inside an
// errors.Join result, as GitHub's per-repo aggregation produces — takes
// priority over the generic Offline bucket, since both carry a more
// specific, more actionable meaning than "something went wrong."
func classifyConnectionState(err error) ConnectionState {
	if err == nil {
		return ConnectionStateOK
	}

	var rateLimitErr adapter.RateLimitError
	var authErr adapter.AuthError
	switch {
	case errors.As(err, &rateLimitErr):
		return ConnectionStateRateLimited
	case errors.As(err, &authErr):
		return ConnectionStateUnauthorized
	default:
		return ConnectionStateOffline
	}
}
