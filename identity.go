package prsm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

// IdentityStatus reports one identity-resolving connection's outcome for the
// ResolveIdentities call that produced the enclosing query.ResolvedIdentities.
//
// It reuses ConnectionState rather than defining its own enum: ResolveIdentity
// hits the same vendor API as ListPullRequests and fails the same three ways —
// offline, rate limited, unauthorized. It is still a distinct type from
// ConnectionStatus, not folded into PullRequestSnapshot, because the two calls
// run on different schedules — identity resolves once at startup,
// ListPullRequests runs every poll cycle — and merging them would tie
// identity's lifetime to Fetch's.
//
// A connection that does not implement adapter.IdentityResolver never
// produces an IdentityStatus at all; see adapter.IdentityResolver's doc
// comment for why that omission, not a degraded status, is how "not served"
// stays distinct from "served, but failed."
type IdentityStatus struct {
	Provider model.ProviderInstance
	State    ConnectionState
	// Err is nil when State is ConnectionStateOK, and the cause otherwise —
	// the same error ResolveIdentity returned, so errors.As still reaches
	// adapter.AuthError / adapter.RateLimitError through it.
	Err error
	// ResolvedAt is when this connection's own ResolveIdentity call returned
	// successfully. It is the zero value on failure, mirroring
	// ConnectionStatus.SucceededAt.
	ResolvedAt time.Time
}

// newIdentityStatus classifies err into a ConnectionState via
// classifyConnectionState and records resolvedAt only on success.
func newIdentityStatus(instance model.ProviderInstance, resolvedAt time.Time, err error) IdentityStatus {
	status := IdentityStatus{Provider: instance, Err: err, State: classifyConnectionState(err)}
	if err == nil {
		status.ResolvedAt = resolvedAt
	}
	return status
}

// ResolveIdentities calls ResolveIdentity on every connection that implements
// adapter.IdentityResolver and aggregates the results into a
// query.ResolvedIdentities map, keyed by ProviderInstance.Name for use with
// query.PRFilterSpec.Compile, plus one IdentityStatus per attempted
// connection recording how the call went.
//
// A connection whose ResolveIdentity call fails contributes no entry to the
// returned map — query.ResolvedIdentities already treats a missing entry as
// "cannot resolve me for this instance," a non-match rather than a match
// against the empty string — but it does get an IdentityStatus reporting the
// failure, so a caller can tell that degraded state apart from a connection
// that never implements adapter.IdentityResolver, which is correctly absent
// from both return values instead of reported as broken.
//
// ResolveIdentities never returns an error itself, mirroring Fetch: a result
// where every connection failed to resolve is still a valid result, reported
// through the returned []IdentityStatus.
//
// Call this once at startup, or again if credentials might rotate at
// runtime; results are not cached on Client, matching Fetch's statelessness —
// a caller that wants to reuse a resolved identity across repeated Fetch
// calls holds onto the returned query.ResolvedIdentities itself.
func (client *Client) ResolveIdentities(ctx context.Context) (query.ResolvedIdentities, []IdentityStatus) {
	identities := make([]*model.Identity, len(client.identityResolvers))
	statuses := make([]IdentityStatus, len(client.identityResolvers))

	var waitGroup sync.WaitGroup
	for index, identityResolver := range client.identityResolvers {
		// Every element of client.identityResolvers was type-asserted from
		// adapter.Connection in NewWithConnections, so this assertion always
		// succeeds; TestNewWithConnections_CapabilityIndexing pins that
		// invariant down.
		connection := identityResolver.(adapter.Connection)

		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					statuses[index] = newIdentityStatus(connection.Instance(), time.Now(), fmt.Errorf("panic: %v", recovered))
				}
			}()

			identity, err := identityResolver.ResolveIdentity(ctx)
			resolvedAt := time.Now()
			statuses[index] = newIdentityStatus(connection.Instance(), resolvedAt, err)
			if err == nil {
				identities[index] = &identity
			}
		}()
	}
	waitGroup.Wait()

	resolvedIdentities := make(query.ResolvedIdentities, len(client.identityResolvers))
	for index, identity := range identities {
		if identity != nil {
			resolvedIdentities[statuses[index].Provider.Name] = *identity
		}
	}

	return resolvedIdentities, statuses
}
