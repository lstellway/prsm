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

func (mockAdapter *MockAdapter) Kind() model.ProviderKind { return mockAdapter.KindVal }

func (mockAdapter *MockAdapter) Instance() model.ProviderInstance { return mockAdapter.InstanceVal }

func (mockAdapter *MockAdapter) ListPullRequests(_ context.Context) ([]model.PullRequest, error) {
	return mockAdapter.PullRequests, mockAdapter.PullRequestsErr
}

func (mockAdapter *MockAdapter) LoadCI(_ context.Context, _ model.PullRequest) (model.CIStatus, error) {
	return mockAdapter.CIStatus, mockAdapter.CIErr
}

func (mockAdapter *MockAdapter) LoadReviewerStates(_ context.Context, _ model.PullRequest) ([]model.ReviewerState, error) {
	return mockAdapter.ReviewerStates, mockAdapter.ReviewerStatesErr
}

func (mockAdapter *MockAdapter) LoadDiff(_ context.Context, _ model.PullRequest) (model.DiffStats, error) {
	return mockAdapter.DiffStats, mockAdapter.DiffErr
}

func (mockAdapter *MockAdapter) ResolveIdentity(_ context.Context) (model.Identity, error) {
	return mockAdapter.Identity, mockAdapter.IdentityErr
}
