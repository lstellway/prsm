package prsm

import (
	"fmt"
	"time"

	"github.com/lstellway/prsm/model"
)

// ProviderPhase distinguishes the two points at which a provider can produce
// a status: before a connection exists at all, or after one has been
// constructed and asked to fetch. It mirrors, one level up, the same
// pre-construction/post-construction split ConstructError and
// ConnectionStatus already keep separate — see ProviderStatus's doc comment
// for why that split is preserved here rather than collapsed.
type ProviderPhase int

const (
	// ProviderPhaseUnknown is the zero value: no phase has been assigned. A
	// ProviderStatus should never carry it — like ConnectionStateUnknown, it
	// exists so a partial literal cannot pass itself off as one of the two
	// real phases.
	ProviderPhaseUnknown ProviderPhase = iota
	// ProviderPhaseConnected means this provider constructed a connection,
	// and ConnectionState/SucceededAt describe the outcome of asking it to
	// fetch this cycle.
	ProviderPhaseConnected
	// ProviderPhaseConstructFailed means this provider never became a
	// connection at all — ConstructReason describes why, and
	// ConnectionState/SucceededAt are meaningless.
	ProviderPhaseConstructFailed
)

func (providerPhase ProviderPhase) String() string {
	switch providerPhase {
	case ProviderPhaseConnected:
		return "ProviderPhaseConnected"
	case ProviderPhaseConstructFailed:
		return "ProviderPhaseConstructFailed"
	default:
		return fmt.Sprintf("ProviderPhase(%d)", int(providerPhase))
	}
}

// ProviderStatus reports what happened with one configured provider,
// regardless of whether it ever became a working connection. It merges the
// two failure sources Client keeps separate — ConnectionStatus (a connection
// was constructed, but this Fetch call was degraded) and ConstructError (the
// provider never became a connection at all) — into the one view every
// consumer needs to answer "why didn't I get pull requests from this
// provider": a TUI status bar, an MCP health resource, and a wire API
// response all want the same merge, so it is computed once here via
// Client.ProviderStatuses rather than re-derived per consumer.
//
// The two causes stay distinguishable through Phase rather than collapsing
// into one flattened state string: a caller that wants to react differently
// to "not supported yet" versus "worth retrying" branches on Phase and the
// matching typed field, and errors.As still reaches adapter.RateLimitError /
// adapter.AuthError through Err. Label renders the right field as text for
// output that only needs a string.
type ProviderStatus struct {
	Provider string
	Kind     model.ProviderKind
	Phase    ProviderPhase

	// ConnectionState is meaningful iff Phase == ProviderPhaseConnected.
	ConnectionState ConnectionState
	// ConstructReason is meaningful iff Phase == ProviderPhaseConstructFailed.
	ConstructReason ConstructErrorReason

	// Err is nil on success, and the cause otherwise: the same error
	// ListPullRequests returned for ProviderPhaseConnected, or the
	// *ConstructError itself for ProviderPhaseConstructFailed — so
	// Err.Error() reads the same as ConstructError.Error()'s formatted
	// message in that case, and errors.As/errors.Unwrap still reach through
	// it either way.
	Err error
	// SucceededAt is meaningful only for ProviderPhaseConnected; see
	// ConnectionStatus.SucceededAt.
	SucceededAt time.Time
}

// Label returns the short, stable, lowercase name a consumer-facing surface
// should print for this status, delegating to whichever phase's own Label is
// meaningful. Only ProviderPhaseUnknown falls through to "unknown", since
// ProviderStatuses never constructs a ProviderStatus with an unset Phase.
func (providerStatus ProviderStatus) Label() string {
	switch providerStatus.Phase {
	case ProviderPhaseConnected:
		return providerStatus.ConnectionState.Label()
	case ProviderPhaseConstructFailed:
		return providerStatus.ConstructReason.Label()
	default:
		return "unknown"
	}
}

// ProviderStatuses merges snapshot.Connections with client.FailedProviders()
// into one per-provider view: connected providers first, in
// snapshot.Connections order, then providers that never constructed, in
// FailedProviders order. snapshot is expected to come from this Client's own
// Fetch; ProviderStatuses does not validate that, the same way Fetch itself
// trusts its caller.
func (client *Client) ProviderStatuses(snapshot PullRequestSnapshot) []ProviderStatus {
	statuses := make([]ProviderStatus, 0, len(snapshot.Connections)+len(client.failedProviders))

	for _, connectionStatus := range snapshot.Connections {
		statuses = append(statuses, ProviderStatus{
			Provider:        connectionStatus.Provider.Name,
			Kind:            connectionStatus.Provider.Kind,
			Phase:           ProviderPhaseConnected,
			ConnectionState: connectionStatus.State,
			Err:             connectionStatus.Err,
			SucceededAt:     connectionStatus.SucceededAt,
		})
	}

	for _, constructError := range client.failedProviders {
		statuses = append(statuses, ProviderStatus{
			Provider:        constructError.Provider,
			Kind:            constructError.Kind,
			Phase:           ProviderPhaseConstructFailed,
			ConstructReason: constructError.Reason,
			Err:             constructError,
		})
	}

	return statuses
}
