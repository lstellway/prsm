// Package prsm is the assembly layer: it turns a loaded (or hand-built)
// config into usable provider connections. New constructs one adapter per
// configured provider and records which resource-kind interfaces the
// resulting connection satisfies, so callers ask "who serves pull requests"
// once at startup instead of probing interfaces at every call site. See
// adapter/adapter.go's package doc for why the interfaces are split per
// resource kind rather than gathered into one.
package prsm

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lstellway/prsm/adapter"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	"github.com/lstellway/prsm/config"
	"github.com/lstellway/prsm/model"
)

// ErrNilConfig is returned by New when prsmConfig is nil. It is the only
// failure New reports through its error return; every other failure is
// per-provider and is recorded on the returned Client instead — see New's
// doc comment for why.
var ErrNilConfig = errors.New("prsm: config is nil")

// ConstructErrorReason distinguishes why a provider failed to construct, so
// callers can react differently — greying out a not-yet-supported vendor
// versus surfacing a bad credential — without parsing error text.
//
// ConstructErrorReasonNotImplemented is a fourth kind of "no," alongside the
// three described in adapter/adapter.go's package doc: those three describe
// what an already-existing connection can answer for a resource kind it
// structurally serves, while NotImplemented fires before any connection
// exists — it means this build of prsm has no adapter package wired up yet
// for a vendor config.LoadFile otherwise accepts. It is a build-completeness
// gap, not a capability-probe result.
type ConstructErrorReason int

const (
	// ConstructErrorReasonUnknownType means ProviderConfig.Type named a
	// vendor prsm has never heard of. Reachable only when a caller
	// hand-builds a config.Config bypassing config.LoadFile's enum check.
	ConstructErrorReasonUnknownType ConstructErrorReason = iota
	// ConstructErrorReasonNotImplemented means the vendor is recognized but
	// its adapter does not exist yet (GitLab, Gitea today). This is
	// temporary, unlike adapter.Connection's structural "not served" case.
	ConstructErrorReasonNotImplemented
	// ConstructErrorReasonDuplicateName means another connection already
	// claimed this provider name — either an earlier entry in
	// prsmConfig.Providers (New), or an earlier connection in the same call
	// (NewWithConnections). Reachable only when a caller bypasses
	// config.LoadFile's uniqueness check or passes overlapping connections
	// directly.
	ConstructErrorReasonDuplicateName
	// ConstructErrorReasonFailed means the vendor's adapter constructor
	// itself returned an error (bad credential, malformed config). Err holds
	// the underlying cause and is reachable via errors.Unwrap.
	ConstructErrorReasonFailed
)

func (reason ConstructErrorReason) String() string {
	switch reason {
	case ConstructErrorReasonUnknownType:
		return "ConstructErrorReasonUnknownType"
	case ConstructErrorReasonNotImplemented:
		return "ConstructErrorReasonNotImplemented"
	case ConstructErrorReasonDuplicateName:
		return "ConstructErrorReasonDuplicateName"
	case ConstructErrorReasonFailed:
		return "ConstructErrorReasonFailed"
	default:
		return fmt.Sprintf("ConstructErrorReason(%d)", int(reason))
	}
}

// ConstructError describes one provider that failed to construct, collected
// on Client.FailedProviders. Its four Reasons are the exhaustive,
// mutually-exclusive output of one function (constructConnection, plus the
// equivalent dedup step in NewWithConnections) and all share the same
// Provider/Kind shape, so they live on one struct with a discriminator
// rather than as four separate types the way adapter/errors.go models
// genuinely independent, heterogeneous adapter-call failures (RateLimitError,
// AuthError, NotFoundError can each occur on any adapter method, at any
// time, with no shared fields). Use errors.As to recover a ConstructError
// from an error value and branch on Reason instead of matching error text.
//
// Kind matches the field name model.ProviderInstance and PullRequestSnapshot's
// ConnectionStatus use for the same concept, rather than the plain-error
// convention of calling it Type.
type ConstructError struct {
	Provider string
	Kind     model.ProviderKind
	Reason   ConstructErrorReason
	Err      error // meaningful only when Reason == ConstructErrorReasonFailed
}

func newConstructError(provider string, providerKind model.ProviderKind, reason ConstructErrorReason, err error) *ConstructError {
	return &ConstructError{Provider: provider, Kind: providerKind, Reason: reason, Err: err}
}

func (constructError *ConstructError) Error() string {
	switch constructError.Reason {
	case ConstructErrorReasonUnknownType:
		return fmt.Sprintf("construct provider %q: unknown provider type %q", constructError.Provider, constructError.Kind)
	case ConstructErrorReasonNotImplemented:
		return fmt.Sprintf("construct provider %q: %q adapter is not implemented yet", constructError.Provider, constructError.Kind)
	case ConstructErrorReasonDuplicateName:
		return fmt.Sprintf("construct provider %q: duplicate provider name", constructError.Provider)
	case ConstructErrorReasonFailed:
		return fmt.Sprintf("construct provider %q: %v", constructError.Provider, constructError.Err)
	default:
		return fmt.Sprintf("construct provider %q: invalid %s", constructError.Provider, constructError.Reason)
	}
}

// Unwrap exposes the underlying adapter error, if any, so errors.Is/errors.As
// can reach it from a ConstructError.
func (constructError *ConstructError) Unwrap() error { return constructError.Err }

// Client aggregates a set of provider connections, indexed by the
// resource-kind interfaces each one satisfies. The zero value is not usable;
// construct via New or NewWithConnections.
//
// A *Client is immutable after construction, and its accessors return cloned
// slices rather than internal state, so concurrent reads from multiple
// goroutines are safe.
type Client struct {
	connections        []adapter.Connection
	pullRequestSources []adapter.PullRequestSource
	identityResolvers  []adapter.IdentityResolver
	failedProviders    []*ConstructError
}

// Connections returns every connection the Client holds, regardless of what
// resource kinds it serves.
func (client *Client) Connections() []adapter.Connection {
	return slices.Clone(client.connections)
}

// PullRequestSources returns the connections that satisfy
// adapter.PullRequestSource.
func (client *Client) PullRequestSources() []adapter.PullRequestSource {
	return slices.Clone(client.pullRequestSources)
}

// IdentityResolvers returns the connections that satisfy
// adapter.IdentityResolver.
func (client *Client) IdentityResolvers() []adapter.IdentityResolver {
	return slices.Clone(client.identityResolvers)
}

// FailedProviders returns one *ConstructError per provider that could not be
// constructed or indexed, in construction order. This is the only place
// per-provider failures are reported — see New's doc comment for why they do
// not also surface through New's returned error.
func (client *Client) FailedProviders() []*ConstructError {
	return slices.Clone(client.failedProviders)
}

// PullRequestSourceFor returns the connection identified by instance.Name, if
// one exists and serves pull requests. Name is the stable identifier every
// model.PullRequest.Provider carries back to its source; both New and
// NewWithConnections reject duplicate names — recording the loser on
// FailedProviders rather than indexing it — so at most one connection can
// match, regardless of which constructor built the Client.
func (client *Client) PullRequestSourceFor(instance model.ProviderInstance) (adapter.PullRequestSource, bool) {
	for _, pullRequestSource := range client.pullRequestSources {
		if pullRequestSource.Instance().Name == instance.Name {
			return pullRequestSource, true
		}
	}
	return nil, false
}

// New constructs a Client from prsmConfig, one connection per entry in
// prsmConfig.Providers. A provider that fails to construct — a missing
// credential, a duplicate name, or a vendor whose adapter is not implemented
// yet — does not abort the call, and does not surface through New's returned
// error either: every failure is recorded on the returned Client, retrieved
// via Client.FailedProviders. New's error return is reserved for failures
// that make construction impossible altogether (today, only prsmConfig being
// nil). This split is deliberate: the ordinary Go idiom
// `client, err := prsm.New(cfg); if err != nil { return err }` must not
// silently discard a Client that is degraded but still usable for every
// provider that DID construct — which a non-nil error on partial failure
// would invite.
//
// prsmConfig is not assumed to have passed through config.LoadFile — a Go
// library caller may hand-construct one directly — so an unrecognized
// provider Type or a duplicate provider Name produces a ConstructError here
// rather than a panic or a silently skipped entry.
func New(prsmConfig *config.Config) (*Client, error) {
	if prsmConfig == nil {
		return &Client{}, ErrNilConfig
	}

	var connections []adapter.Connection
	var failedProviders []*ConstructError

	seenProviderNames := make(map[string]bool, len(prsmConfig.Providers))
	for _, providerConfig := range prsmConfig.Providers {
		// A name already claimed by an earlier entry is rejected here, before
		// construction is attempted, even if that earlier entry itself went
		// on to fail. The name slot is claimed by config order, not by
		// construction success.
		if seenProviderNames[providerConfig.Name] {
			failedProviders = append(failedProviders, newConstructError(
				providerConfig.Name, providerConfig.Type, ConstructErrorReasonDuplicateName, nil,
			))
			continue
		}
		seenProviderNames[providerConfig.Name] = true

		connection, constructError := constructConnection(providerConfig)
		if constructError != nil {
			failedProviders = append(failedProviders, constructError)
			continue
		}
		connections = append(connections, connection)
	}

	client := NewWithConnections(connections...)
	client.failedProviders = append(failedProviders, client.failedProviders...)
	return client, nil
}

// constructConnection maps one config.ProviderConfig into its vendor's own
// Config type and constructs that vendor's adapter. GitLab and Gitea are not
// implemented yet: naming either type is a construction error, not a panic,
// not a silent skip. An unrecognized Type — reachable only when a caller
// bypasses config.LoadFile's enum validation — is handled the same way.
func constructConnection(providerConfig config.ProviderConfig) (adapter.Connection, *ConstructError) {
	switch providerConfig.Type {
	case model.ProviderGitHub:
		githubAdapter, err := adaptergithub.New(githubConfig(providerConfig))
		if err != nil {
			return nil, newConstructError(providerConfig.Name, providerConfig.Type, ConstructErrorReasonFailed, err)
		}
		return githubAdapter, nil
	case model.ProviderGitLab, model.ProviderGitea:
		return nil, newConstructError(providerConfig.Name, providerConfig.Type, ConstructErrorReasonNotImplemented, nil)
	default:
		return nil, newConstructError(providerConfig.Name, providerConfig.Type, ConstructErrorReasonUnknownType, nil)
	}
}

// NewWithConnections builds a Client directly from already-constructed
// connections, bypassing config entirely. It shares New's capability-
// indexing logic — type-asserting each connection against
// adapter.PullRequestSource and adapter.IdentityResolver — plus New's
// duplicate-name handling: if two connections report the same
// Instance().Name, the first is indexed and later ones are recorded on
// FailedProviders with Reason ConstructErrorReasonDuplicateName instead, so
// PullRequestSourceFor's "at most one match" guarantee holds no matter which
// constructor built the Client. Used by tests and library callers holding
// connections directly, e.g. from adapter/mock.
func NewWithConnections(connections ...adapter.Connection) *Client {
	client := &Client{}

	seenNames := make(map[string]bool, len(connections))
	for _, connection := range connections {
		instance := connection.Instance()
		if seenNames[instance.Name] {
			client.failedProviders = append(client.failedProviders, newConstructError(
				instance.Name, instance.Kind, ConstructErrorReasonDuplicateName, nil,
			))
			continue
		}
		seenNames[instance.Name] = true

		client.connections = append(client.connections, connection)
		if pullRequestSource, ok := connection.(adapter.PullRequestSource); ok {
			client.pullRequestSources = append(client.pullRequestSources, pullRequestSource)
		}
		if identityResolver, ok := connection.(adapter.IdentityResolver); ok {
			client.identityResolvers = append(client.identityResolvers, identityResolver)
		}
	}

	return client
}
