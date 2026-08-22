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
)

// Client aggregates a set of provider connections, indexed by the
// resource-kind interfaces each one satisfies. The zero value is not usable;
// construct via New or NewWithAdapters.
type Client struct {
	connections        []adapter.Connection
	pullRequestSources []adapter.PullRequestSource
	identityResolvers  []adapter.IdentityResolver
}

// Connections returns every connection the Client holds, regardless of what
// resource kinds it serves.
func (client *Client) Connections() []adapter.Connection { return client.connections }

// PullRequestSources returns the connections that satisfy
// adapter.PullRequestSource.
func (client *Client) PullRequestSources() []adapter.PullRequestSource {
	return client.pullRequestSources
}

// IdentityResolvers returns the connections that satisfy
// adapter.IdentityResolver.
func (client *Client) IdentityResolvers() []adapter.IdentityResolver {
	return client.identityResolvers
}

// New constructs a Client from prsmConfig, one connection per entry in
// prsmConfig.Providers. A provider that fails to construct — a bad or
// missing credential, or a vendor whose adapter is not implemented yet —
// does not abort the call: New returns a *Client holding every connection
// that DID construct, plus a non-nil error joining one failure per bad
// provider via errors.Join. This mirrors
// adapter/github.GitHubAdapter.ListPullRequests, which returns partial
// results alongside errors.Join(fetchErrors...) rather than aborting on the
// first bad repo. The returned *Client is never nil, even when every
// provider fails; callers must check the error to detect degraded state.
//
// prsmConfig is not assumed to have passed through config.LoadFile — a Go
// library caller may hand-construct one directly — so an unrecognized
// provider Type produces a construction error here rather than a panic or a
// silently skipped entry.
func New(prsmConfig *config.Config) (*Client, error) {
	if prsmConfig == nil {
		return &Client{}, fmt.Errorf("prsm: config is nil")
	}

	var connections []adapter.Connection
	var constructionErrors []error

	for _, providerConfig := range prsmConfig.Providers {
		connection, err := constructConnection(providerConfig)
		if err != nil {
			constructionErrors = append(constructionErrors, err)
			continue
		}
		connections = append(connections, connection)
	}

	return NewWithAdapters(connections...), errors.Join(constructionErrors...)
}

// constructConnection maps one config.ProviderConfig into its vendor's own
// Config type and constructs that vendor's adapter. GitLab and Gitea are not
// implemented yet: naming either type is a construction error, not a panic,
// not a silent skip. An unrecognized Type — reachable only when a caller
// bypasses config.LoadFile's enum validation — is handled the same way.
func constructConnection(providerConfig config.ProviderConfig) (adapter.Connection, error) {
	switch providerConfig.Type {
	case "github":
		githubAdapter, err := adaptergithub.New(githubConfig(providerConfig))
		if err != nil {
			return nil, fmt.Errorf("construct provider %q: %w", providerConfig.Name, err)
		}
		return githubAdapter, nil
	case "gitlab":
		return nil, fmt.Errorf("construct provider %q: gitlab adapter is not implemented yet", providerConfig.Name)
	case "gitea":
		return nil, fmt.Errorf("construct provider %q: gitea adapter is not implemented yet", providerConfig.Name)
	default:
		return nil, fmt.Errorf("construct provider %q: unknown provider type %q", providerConfig.Name, providerConfig.Type)
	}
}

// NewWithAdapters builds a Client directly from already-constructed
// connections, bypassing config entirely. It shares New's capability-
// indexing logic — type-asserting each connection against
// adapter.PullRequestSource and adapter.IdentityResolver — and cannot fail:
// indexing is pure introspection over values the caller already built. Used
// by tests and library callers holding adapters directly, e.g. from
// adapter/mock.
func NewWithAdapters(connections ...adapter.Connection) *Client {
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
