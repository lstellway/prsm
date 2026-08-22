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

// ErrNilConfig is returned by New when prsmConfig is nil.
var ErrNilConfig = errors.New("prsm: config is nil")

// ConstructErrorReason distinguishes why a provider failed to construct, so
// callers can react differently — greying out a not-yet-supported vendor
// versus surfacing a bad credential — without parsing error text.
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
	// ConstructErrorReasonDuplicateName means an earlier entry in
	// prsmConfig.Providers already used this name. Reachable only when a
	// caller hand-builds a config.Config bypassing config.LoadFile's
	// uniqueness check.
	ConstructErrorReasonDuplicateName
	// ConstructErrorReasonFailed means the vendor's adapter constructor
	// itself returned an error (bad credential, malformed config). Err holds
	// the underlying cause and is reachable via errors.Unwrap.
	ConstructErrorReasonFailed
)

// ConstructError describes one provider that failed to construct. New
// returns these wrapped inside its errors.Join tree; use errors.As to
// recover one and branch on Reason instead of matching error text.
type ConstructError struct {
	Provider string
	Type     model.ProviderKind
	Reason   ConstructErrorReason
	Err      error // set only when Reason == ConstructErrorReasonFailed
}

func (constructError *ConstructError) Error() string {
	switch constructError.Reason {
	case ConstructErrorReasonNotImplemented:
		return fmt.Sprintf("construct provider %q: %s adapter is not implemented yet", constructError.Provider, constructError.Type)
	case ConstructErrorReasonDuplicateName:
		return fmt.Sprintf("construct provider %q: duplicate provider name", constructError.Provider)
	case ConstructErrorReasonFailed:
		return fmt.Sprintf("construct provider %q: %v", constructError.Provider, constructError.Err)
	default:
		return fmt.Sprintf("construct provider %q: unknown provider type %q", constructError.Provider, constructError.Type)
	}
}

// Unwrap exposes the underlying adapter error, if any, so errors.Is/errors.As
// can reach it through New's errors.Join tree.
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

// FailedProviders returns one *ConstructError per provider that New could not
// construct, in config order. Empty for a Client built via NewWithConnections,
// which has no construction step to fail.
func (client *Client) FailedProviders() []*ConstructError {
	return slices.Clone(client.failedProviders)
}

// PullRequestSourceFor returns the connection identified by instance.Name, if
// one exists and serves pull requests. Name is the stable identifier every
// model.PullRequest.Provider carries back to its source, and New rejects
// duplicate provider names, so at most one connection can match.
func (client *Client) PullRequestSourceFor(instance model.ProviderInstance) (adapter.PullRequestSource, bool) {
	for _, pullRequestSource := range client.pullRequestSources {
		if pullRequestSource.Instance().Name == instance.Name {
			return pullRequestSource, true
		}
	}
	return nil, false
}

// New constructs a Client from prsmConfig, one connection per entry in
// prsmConfig.Providers. A provider that fails to construct — a bad or
// missing credential, a duplicate name, or a vendor whose adapter is not
// implemented yet — does not abort the call: New returns a *Client holding
// every connection that DID construct, plus a non-nil error joining one
// *ConstructError per bad provider via errors.Join. This mirrors
// adapter/github.GitHubAdapter.ListPullRequests, which returns partial
// results alongside errors.Join(fetchErrors...) rather than aborting on the
// first bad repo. The returned *Client is never nil, even when every
// provider fails; callers must check the error, or call FailedProviders, to
// detect degraded state.
//
// prsmConfig is not assumed to have passed through config.LoadFile — a Go
// library caller may hand-construct one directly — so an unrecognized
// provider Type or a duplicate provider Name produces a construction error
// here rather than a panic or a silently skipped entry.
func New(prsmConfig *config.Config) (*Client, error) {
	if prsmConfig == nil {
		return &Client{}, ErrNilConfig
	}

	var connections []adapter.Connection
	var constructionErrors []error
	var failedProviders []*ConstructError

	seenProviderNames := make(map[string]bool, len(prsmConfig.Providers))
	for _, providerConfig := range prsmConfig.Providers {
		if seenProviderNames[providerConfig.Name] {
			constructError := &ConstructError{
				Provider: providerConfig.Name,
				Type:     providerConfig.Type,
				Reason:   ConstructErrorReasonDuplicateName,
			}
			constructionErrors = append(constructionErrors, constructError)
			failedProviders = append(failedProviders, constructError)
			continue
		}
		seenProviderNames[providerConfig.Name] = true

		connection, constructError := constructConnection(providerConfig)
		if constructError != nil {
			constructionErrors = append(constructionErrors, constructError)
			failedProviders = append(failedProviders, constructError)
			continue
		}
		connections = append(connections, connection)
	}

	client := NewWithConnections(connections...)
	client.failedProviders = failedProviders
	return client, errors.Join(constructionErrors...)
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
			return nil, &ConstructError{
				Provider: providerConfig.Name,
				Type:     providerConfig.Type,
				Reason:   ConstructErrorReasonFailed,
				Err:      err,
			}
		}
		return githubAdapter, nil
	case model.ProviderGitLab, model.ProviderGitea:
		return nil, &ConstructError{
			Provider: providerConfig.Name,
			Type:     providerConfig.Type,
			Reason:   ConstructErrorReasonNotImplemented,
		}
	default:
		return nil, &ConstructError{
			Provider: providerConfig.Name,
			Type:     providerConfig.Type,
			Reason:   ConstructErrorReasonUnknownType,
		}
	}
}

// NewWithConnections builds a Client directly from already-constructed
// connections, bypassing config entirely. It shares New's capability-
// indexing logic — type-asserting each connection against
// adapter.PullRequestSource and adapter.IdentityResolver — and cannot fail:
// indexing is pure introspection over values the caller already built. Used
// by tests and library callers holding connections directly, e.g. from
// adapter/mock.
func NewWithConnections(connections ...adapter.Connection) *Client {
	client := &Client{connections: slices.Clone(connections)}

	for _, connection := range connections {
		if pullRequestSource, ok := connection.(adapter.PullRequestSource); ok {
			client.pullRequestSources = append(client.pullRequestSources, pullRequestSource)
		}
		if identityResolver, ok := connection.(adapter.IdentityResolver); ok {
			client.identityResolvers = append(client.identityResolvers, identityResolver)
		}
	}

	return client
}
