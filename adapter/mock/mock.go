// Package mock provides in-memory adapter implementations for tests.
//
// The types here mirror the adapter interfaces one-for-one and compose by
// embedding, so a test builds a connection serving exactly the resource kinds
// it means to exercise — including none. A single struct implementing every
// method could not express a source that does not serve pull requests, which is
// the case the split exists to represent.
package mock

import (
	"context"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// Connection is a source that names its instance and serves nothing else. Use
// it alone to stand in for a connection with no credential and no resource
// kinds, or embed it in a larger mock.
type Connection struct {
	InstanceVal model.ProviderInstance
}

func (mockConnection *Connection) Instance() model.ProviderInstance {
	return mockConnection.InstanceVal
}

// PullRequestSource is a Connection that serves pull requests. Set the
// appropriate fields to control return values.
type PullRequestSource struct {
	Connection

	// Delay, when nonzero, makes ListPullRequests sleep before returning —
	// for tests that need to observe real concurrent timing (e.g. that
	// several connections are actually fanned out in parallel, not called
	// one after another).
	Delay             time.Duration
	PullRequests      []model.PullRequest
	PullRequestsErr   error
	CIStatus          model.CIStatus
	CIErr             error
	ReviewerStates    []model.ReviewerState
	ReviewerStatesErr error
	DiffStats         model.DiffStats
	DiffErr           error
}

func (mockSource *PullRequestSource) ListPullRequests(_ context.Context) ([]model.PullRequest, error) {
	if mockSource.Delay > 0 {
		time.Sleep(mockSource.Delay)
	}
	return mockSource.PullRequests, mockSource.PullRequestsErr
}

func (mockSource *PullRequestSource) LoadCI(_ context.Context, _ model.PullRequestRef) (model.CIStatus, error) {
	return mockSource.CIStatus, mockSource.CIErr
}

func (mockSource *PullRequestSource) LoadReviewerStates(_ context.Context, _ model.PullRequestRef) ([]model.ReviewerState, error) {
	return mockSource.ReviewerStates, mockSource.ReviewerStatesErr
}

func (mockSource *PullRequestSource) LoadDiff(_ context.Context, _ model.PullRequestRef) (model.DiffStats, error) {
	return mockSource.DiffStats, mockSource.DiffErr
}

// IdentityResolver resolves an identity. It carries no Connection of its own —
// adapter.IdentityResolver now requires one — so it must always be embedded
// beside a Connection (see Adapter) rather than used standalone.
type IdentityResolver struct {
	// Delay, when nonzero, makes ResolveIdentity sleep before returning — for
	// tests that need to observe real concurrent timing, mirroring
	// PullRequestSource.Delay.
	Delay       time.Duration
	Identity    model.Identity
	IdentityErr error
}

func (mockResolver *IdentityResolver) ResolveIdentity(_ context.Context) (model.Identity, error) {
	if mockResolver.Delay > 0 {
		time.Sleep(mockResolver.Delay)
	}
	return mockResolver.Identity, mockResolver.IdentityErr
}

// Adapter is a connection serving every kind this package mocks — a credentialed
// pull request source, the shape the GitHub adapter has. Tests that do not care
// about the resource-kind split should use this one.
type Adapter struct {
	PullRequestSource
	IdentityResolver
}

var (
	_ adapter.Connection        = (*Connection)(nil)
	_ adapter.PullRequestSource = (*PullRequestSource)(nil)
	_ adapter.PullRequestSource = (*Adapter)(nil)
	_ adapter.IdentityResolver  = (*Adapter)(nil)
)
