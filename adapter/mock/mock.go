package mock

import (
	"context"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// MockAdapter is a configurable ProviderAdapter for use in tests.
// Set the appropriate fields to control return values.
type MockAdapter struct {
	KindVal           model.ProviderKind
	InstanceVal       model.ProviderInstance
	PullRequests      []model.PullRequest
	PullRequestsErr   error
	CIStatus          model.CIStatus
	CIErr             error
	ReviewerStates    []model.ReviewerState
	ReviewerStatesErr error
	DiffStats         model.DiffStats
	DiffErr           error
	Identity          model.Identity
	IdentityErr       error
}

var _ adapter.ProviderAdapter = &MockAdapter{}

func (m *MockAdapter) Kind() model.ProviderKind { return m.KindVal }

func (m *MockAdapter) Instance() model.ProviderInstance { return m.InstanceVal }

func (m *MockAdapter) ListPullRequests(_ context.Context) ([]model.PullRequest, error) {
	return m.PullRequests, m.PullRequestsErr
}

func (m *MockAdapter) LoadCI(_ context.Context, _ model.PullRequest) (model.CIStatus, error) {
	return m.CIStatus, m.CIErr
}

func (m *MockAdapter) LoadReviewerStates(_ context.Context, _ model.PullRequest) ([]model.ReviewerState, error) {
	return m.ReviewerStates, m.ReviewerStatesErr
}

func (m *MockAdapter) LoadDiff(_ context.Context, _ model.PullRequest) (model.DiffStats, error) {
	return m.DiffStats, m.DiffErr
}

func (m *MockAdapter) ResolveIdentity(_ context.Context) (model.Identity, error) {
	return m.Identity, m.IdentityErr
}
