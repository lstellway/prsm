package prsm

import (
	"context"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

// IdentityStatus reports one identity-resolving connection's outcome for the
// ResolveIdentities call that produced the enclosing IdentityResolution.
//
// It shares connectionOutcome with ConnectionStatus rather than repeating
// Provider/State/Err on its own: ResolveIdentity hits the same vendor API as
// ListPullRequests and fails the same three ways — offline, rate limited,
// unauthorized. IdentityStatus stays a distinct named type, not folded into
// ConnectionStatus itself, because the two calls run on different
// schedules — identity resolves once at startup, ListPullRequests runs every
// poll cycle — and giving them one type would tie identity's lifetime to
// Fetch's.
//
// A connection that does not implement adapter.IdentityResolver never
// produces an IdentityStatus at all; see adapter.IdentityResolver's doc
// comment for why that omission, not a degraded status, is how "not served"
// stays distinct from "served, but failed."
type IdentityStatus struct {
	connectionOutcome
	// ResolvedAt is when this connection's own ResolveIdentity call returned
	// successfully. It is the zero value on failure, mirroring
	// ConnectionStatus.SucceededAt.
	ResolvedAt time.Time
}

// newIdentityStatus classifies err into a ConnectionState via
// classifyConnectionState and records resolvedAt only on success.
func newIdentityStatus(instance model.ProviderInstance, resolvedAt time.Time, err error) IdentityStatus {
	status := IdentityStatus{connectionOutcome: connectionOutcome{Provider: instance, Err: err, State: classifyConnectionState(err)}}
	if err == nil {
		status.ResolvedAt = resolvedAt
	}
	return status
}

// IdentityResolution is one point-in-time result of resolving "me" across
// every connection a Client holds, mirroring PullRequestSnapshot's shape for
// Fetch. It carries no memory of any earlier IdentityResolution — see
// IdentityStatus.ResolvedAt.
type IdentityResolution struct {
	// Identities maps provider instance name to the resolved identity, ready
	// to use as query.PRFilterSpec.Compile's resolvedMe argument. A
	// connection whose resolution failed contributes no entry here — see
	// Statuses for that connection's degraded status.
	Identities query.ResolvedIdentities
	// Statuses reports one status per connection ResolveIdentities attempted,
	// in Client.identityResolvers order.
	Statuses []IdentityStatus
	// ResolvedAt is when this IdentityResolution's ResolveIdentities call was made.
	ResolvedAt time.Time
}

// ResolveIdentities calls ResolveIdentity on every connection that implements
// adapter.IdentityResolver and aggregates the results into an
// IdentityResolution: a query.ResolvedIdentities map, keyed by
// ProviderInstance.Name for use with query.PRFilterSpec.Compile, plus one
// IdentityStatus per attempted connection recording how the call went.
//
// A connection whose ResolveIdentity call fails contributes no entry to
// Identities — query.ResolvedIdentities already treats a missing entry as
// "cannot resolve me for this instance," a non-match rather than a match
// against the empty string — but it does get an IdentityStatus reporting the
// failure, so a caller can tell that degraded state apart from a connection
// that never implements adapter.IdentityResolver, which is correctly absent
// from both Identities and Statuses instead of reported as broken.
//
// ResolveIdentities never returns an error itself, mirroring Fetch: a result
// where every connection failed to resolve is still a valid result, reported
// through the returned IdentityResolution.
//
// Call this once at startup, or again if credentials might rotate at
// runtime; results are not cached on Client, matching Fetch's statelessness —
// a caller that wants to reuse a resolved identity across repeated Fetch
// calls holds onto the returned IdentityResolution itself.
func (client *Client) ResolveIdentities(ctx context.Context) IdentityResolution {
	resolvedAt := time.Now()

	identities, statuses := fanOut(ctx, client.identityResolvers,
		func(ctx context.Context, identityResolver adapter.IdentityResolver) (*model.Identity, error) {
			identity, err := identityResolver.ResolveIdentity(ctx)
			if err != nil {
				return nil, err
			}
			return &identity, nil
		},
		newIdentityStatus,
	)

	resolvedIdentities := make(query.ResolvedIdentities, len(client.identityResolvers))
	for index, identity := range identities {
		if identity != nil {
			resolvedIdentities[statuses[index].Provider.Name] = *identity
		}
	}

	return IdentityResolution{
		Identities: resolvedIdentities,
		Statuses:   statuses,
		ResolvedAt: resolvedAt,
	}
}
