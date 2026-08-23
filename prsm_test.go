package prsm

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/config"
	"github.com/lstellway/prsm/model"
)

// identityOnlyConnection is a connection that resolves an identity but does
// not serve pull requests. adapter/mock has no ready-made type for this
// quadrant: mock.IdentityResolver carries no Connection of its own so that it
// composes with any source, so a connection needs one embedded beside it.
type identityOnlyConnection struct {
	mock.Connection
	mock.IdentityResolver
}

var (
	_ adapter.Connection       = (*identityOnlyConnection)(nil)
	_ adapter.IdentityResolver = (*identityOnlyConnection)(nil)
)

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_AllSuccessGitHubOnly(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{Token: "ghp_test"},
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1", len(client.Connections()))
	}
	if len(client.PullRequestSources()) != 1 {
		t.Errorf("PullRequestSources() = %d, want 1", len(client.PullRequestSources()))
	}
	if len(client.IdentityResolvers()) != 1 {
		t.Errorf("IdentityResolvers() = %d, want 1", len(client.IdentityResolvers()))
	}
	if len(client.FailedProviders()) != 0 {
		t.Errorf("FailedProviders() = %d, want 0", len(client.FailedProviders()))
	}
	if instance := client.Connections()[0].Instance(); instance.Kind != model.ProviderGitHub {
		t.Errorf("Instance().Kind = %v, want %v", instance.Kind, model.ProviderGitHub)
	}
}

func TestNew_PartialFailureGitHubAndGitLab(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{Token: "ghp_test"},
			},
			{
				Name: "gitlab-work",
				Type: "gitlab",
				Auth: config.AuthConfig{Token: "glpat_test"},
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil: per-provider failures surface via FailedProviders, not err", err)
	}

	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1", len(client.Connections()))
	}
	if instance := client.Connections()[0].Instance(); instance.Name != "github-personal" {
		t.Errorf("surviving connection Instance().Name = %q, want %q", instance.Name, "github-personal")
	}
	if len(client.PullRequestSources()) != 1 {
		t.Errorf("PullRequestSources() = %d, want 1", len(client.PullRequestSources()))
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}
	if failedProviders[0].Provider != "gitlab-work" || failedProviders[0].Reason != ConstructErrorReasonNotImplemented {
		t.Errorf("FailedProviders()[0] = %+v, want Provider=gitlab-work Reason=ConstructErrorReasonNotImplemented", failedProviders[0])
	}
}

func TestNew_MultipleSimultaneousFailures(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{Token: "ghp_test"},
			},
			{
				Name: "gitlab-work",
				Type: "gitlab",
				Auth: config.AuthConfig{Token: "glpat_test"},
			},
			{
				Name: "gitea-home",
				Type: "gitea",
				Auth: config.AuthConfig{Token: "gitea_test"},
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 2 {
		t.Fatalf("FailedProviders() = %d, want 2 (both failures retained, not just the first)", len(failedProviders))
	}
	failedNames := []string{failedProviders[0].Provider, failedProviders[1].Provider}
	if want := []string{"gitlab-work", "gitea-home"}; !reflect.DeepEqual(failedNames, want) {
		t.Errorf("FailedProviders() names = %v, want %v", failedNames, want)
	}

	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1", len(client.Connections()))
	}
}

func TestNew_ConstructorFailureIsRecordedAndWrapped(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{}, // no token: adaptergithub.New will fail
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}

	failedProvider := failedProviders[0]
	if failedProvider.Reason != ConstructErrorReasonFailed {
		t.Errorf("Reason = %v, want ConstructErrorReasonFailed", failedProvider.Reason)
	}
	if failedProvider.Err == nil {
		t.Fatal("Err = nil, want the underlying adaptergithub.New error")
	}
	if !strings.Contains(failedProvider.Err.Error(), "token") {
		t.Errorf("Err = %q, want it to mention the missing token", failedProvider.Err.Error())
	}
	if !strings.Contains(failedProvider.Error(), "github-personal") {
		t.Errorf("Error() = %q, want it to name the provider", failedProvider.Error())
	}
	if unwrapped := errors.Unwrap(failedProvider); unwrapped != failedProvider.Err {
		t.Errorf("errors.Unwrap(failedProvider) = %v, want %v", unwrapped, failedProvider.Err)
	}
}

func TestNew_AllFailureGitLabOnly(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "gitlab-work",
				Type: "gitlab",
				Auth: config.AuthConfig{Token: "glpat_test"},
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("New() client = nil, want a non-nil Client even on total failure")
	}
	if len(client.Connections()) != 0 || len(client.PullRequestSources()) != 0 || len(client.IdentityResolvers()) != 0 {
		t.Errorf("client = %+v, want all accessors empty", client)
	}
	if len(client.FailedProviders()) != 1 {
		t.Errorf("FailedProviders() = %d, want 1", len(client.FailedProviders()))
	}
}

func TestNew_UnknownProviderType(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "bitbucket-thing", Type: "bitbucket"},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if len(client.Connections()) != 0 {
		t.Errorf("Connections() = %d, want 0", len(client.Connections()))
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}
	if failedProviders[0].Reason != ConstructErrorReasonUnknownType {
		t.Errorf("Reason = %v, want ConstructErrorReasonUnknownType", failedProviders[0].Reason)
	}
	if !strings.Contains(failedProviders[0].Error(), "bitbucket") {
		t.Errorf("Error() = %q, want it to mention the unknown type", failedProviders[0].Error())
	}
}

func TestNew_DuplicateProviderName(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{Token: "ghp_test"},
			},
			{
				Name: "github-personal",
				Type: "github",
				Auth: config.AuthConfig{Token: "ghp_test_2"},
			},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}
	if failedProviders[0].Reason != ConstructErrorReasonDuplicateName {
		t.Errorf("Reason = %v, want ConstructErrorReasonDuplicateName", failedProviders[0].Reason)
	}

	// The first occurrence still constructs; only the repeat is rejected.
	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1", len(client.Connections()))
	}
}

func TestNew_DuplicateName_FirstFailsSecondWouldSucceed(t *testing.T) {
	// A name is claimed by config order, not by construction success: the
	// first entry named "x" fails to construct (gitlab isn't implemented),
	// but that still blocks the second entry of the same name from being
	// attempted, even though it carries a valid github token and would have
	// succeeded on its own.
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "x", Type: "gitlab", Auth: config.AuthConfig{Token: "glpat_test"}},
			{Name: "x", Type: "github", Auth: config.AuthConfig{Token: "ghp_test"}},
		},
	}

	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if len(client.Connections()) != 0 {
		t.Fatalf("Connections() = %d, want 0: the name slot was claimed by the first (failed) entry", len(client.Connections()))
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 2 {
		t.Fatalf("FailedProviders() = %d, want 2", len(failedProviders))
	}
	if failedProviders[0].Reason != ConstructErrorReasonNotImplemented {
		t.Errorf("FailedProviders()[0].Reason = %v, want ConstructErrorReasonNotImplemented", failedProviders[0].Reason)
	}
	if failedProviders[1].Reason != ConstructErrorReasonDuplicateName {
		t.Errorf("FailedProviders()[1].Reason = %v, want ConstructErrorReasonDuplicateName", failedProviders[1].Reason)
	}
}

func TestNew_EmptyProviders(t *testing.T) {
	client, err := New(&config.Config{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("New() client = nil, want a non-nil Client")
	}
	if len(client.Connections()) != 0 || len(client.PullRequestSources()) != 0 ||
		len(client.IdentityResolvers()) != 0 || len(client.FailedProviders()) != 0 {
		t.Errorf("client = %+v, want all accessors empty", client)
	}
}

func TestNew_NilConfig(t *testing.T) {
	client, err := New(nil)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("New(nil) error = %v, want ErrNilConfig", err)
	}
	if client == nil {
		t.Fatal("New(nil) client = nil, want a non-nil Client")
	}
	if len(client.Connections()) != 0 {
		t.Errorf("Connections() = %d, want 0", len(client.Connections()))
	}
}

// ---------------------------------------------------------------------------
// NewWithConnections
// ---------------------------------------------------------------------------

func TestNewWithConnections_CapabilityIndexing(t *testing.T) {
	neither := &mock.Connection{InstanceVal: model.ProviderInstance{Name: "neither"}}
	pullRequestOnly := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "pr-only"}},
	}
	identityOnly := &identityOnlyConnection{
		Connection:       mock.Connection{InstanceVal: model.ProviderInstance{Name: "identity-only"}},
		IdentityResolver: mock.IdentityResolver{Identity: model.Identity{Username: "alice"}},
	}
	both := &mock.Adapter{
		PullRequestSource: mock.PullRequestSource{
			Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "both"}},
		},
	}

	client := NewWithConnections(neither, pullRequestOnly, identityOnly, both)

	if len(client.Connections()) != 4 {
		t.Fatalf("Connections() = %d, want 4", len(client.Connections()))
	}
	if len(client.FailedProviders()) != 0 {
		t.Errorf("FailedProviders() = %d, want 0: no names collide", len(client.FailedProviders()))
	}
	for index, wantName := range []string{"neither", "pr-only", "identity-only", "both"} {
		if got := client.Connections()[index].Instance().Name; got != wantName {
			t.Errorf("Connections()[%d].Instance().Name = %q, want %q", index, got, wantName)
		}
	}

	// Indexing preserves the order connections were passed in, so the
	// expected names below are exact, not just a set.
	pullRequestNames := make([]string, len(client.PullRequestSources()))
	for index, pullRequestSource := range client.PullRequestSources() {
		pullRequestNames[index] = pullRequestSource.Instance().Name
	}
	if want := []string{"pr-only", "both"}; !reflect.DeepEqual(pullRequestNames, want) {
		t.Errorf("PullRequestSources() names = %v, want %v", pullRequestNames, want)
	}

	identityNames := make([]string, len(client.IdentityResolvers()))
	for index, identityResolver := range client.IdentityResolvers() {
		connection, ok := identityResolver.(adapter.Connection)
		if !ok {
			t.Fatalf("IdentityResolvers()[%d] does not implement adapter.Connection", index)
		}
		identityNames[index] = connection.Instance().Name
	}
	if want := []string{"identity-only", "both"}; !reflect.DeepEqual(identityNames, want) {
		t.Errorf("IdentityResolvers() names = %v, want %v", identityNames, want)
	}

	pullRequestSource, ok := client.PullRequestSourceFor(model.ProviderInstance{Name: "both"})
	if !ok {
		t.Fatal("PullRequestSourceFor(both) ok = false, want true")
	}
	if pullRequestSource.Instance().Name != "both" {
		t.Errorf("PullRequestSourceFor(both).Instance().Name = %q, want %q", pullRequestSource.Instance().Name, "both")
	}

	if _, ok := client.PullRequestSourceFor(model.ProviderInstance{Name: "identity-only"}); ok {
		t.Error("PullRequestSourceFor(identity-only) ok = true, want false: that connection does not serve pull requests")
	}
	if _, ok := client.PullRequestSourceFor(model.ProviderInstance{Name: "does-not-exist"}); ok {
		t.Error("PullRequestSourceFor(does-not-exist) ok = true, want false")
	}
}

func TestNewWithConnections_DuplicateNames(t *testing.T) {
	first := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "dup", Kind: model.ProviderGitHub}},
	}
	second := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "dup", Kind: model.ProviderGitea}},
	}

	client := NewWithConnections(first, second)

	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1: only the first occurrence is indexed", len(client.Connections()))
	}
	if got := client.Connections()[0].Instance().Kind; got != model.ProviderGitHub {
		t.Errorf("surviving connection Kind = %v, want %v (the first occurrence)", got, model.ProviderGitHub)
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}
	if failedProviders[0].Provider != "dup" || failedProviders[0].Kind != model.ProviderGitea || failedProviders[0].Reason != ConstructErrorReasonDuplicateName {
		t.Errorf("FailedProviders()[0] = %+v, want Provider=dup Kind=%v Reason=ConstructErrorReasonDuplicateName", failedProviders[0], model.ProviderGitea)
	}

	pullRequestSource, ok := client.PullRequestSourceFor(model.ProviderInstance{Name: "dup"})
	if !ok {
		t.Fatal("PullRequestSourceFor(dup) ok = false, want true")
	}
	if pullRequestSource.Instance().Kind != model.ProviderGitHub {
		t.Errorf("PullRequestSourceFor(dup).Instance().Kind = %v, want %v (the first occurrence)", pullRequestSource.Instance().Kind, model.ProviderGitHub)
	}
}

func TestNewWithConnections_AccessorsReturnCopies(t *testing.T) {
	original := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "original"}},
	}
	duplicate := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "original"}},
	}
	identityConnection := &identityOnlyConnection{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "identity"}},
	}

	client := NewWithConnections(original, duplicate, identityConnection)

	connections := client.Connections()
	connections[0] = &mock.Connection{InstanceVal: model.ProviderInstance{Name: "mutated"}}
	if got := client.Connections()[0].Instance().Name; got != "original" {
		t.Errorf("Connections()[0] = %q after mutating a returned slice, want %q", got, "original")
	}

	pullRequestSources := client.PullRequestSources()
	pullRequestSources[0] = &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "mutated"}},
	}
	if got := client.PullRequestSources()[0].Instance().Name; got != "original" {
		t.Errorf("PullRequestSources()[0] = %q after mutating a returned slice, want %q", got, "original")
	}

	identityResolvers := client.IdentityResolvers()
	identityResolvers[0] = nil
	if client.IdentityResolvers()[0] == nil {
		t.Error("IdentityResolvers()[0] became nil after mutating a returned slice")
	}

	failedProviders := client.FailedProviders()
	if len(failedProviders) != 1 {
		t.Fatalf("FailedProviders() = %d, want 1", len(failedProviders))
	}
	failedProviders[0] = &ConstructError{Provider: "mutated"}
	if got := client.FailedProviders()[0].Provider; got != "original" {
		t.Errorf("FailedProviders()[0].Provider = %q after mutating a returned slice, want %q", got, "original")
	}
}

func TestNewWithConnections_Empty(t *testing.T) {
	client := NewWithConnections()
	if client == nil {
		t.Fatal("NewWithConnections() = nil, want a non-nil Client")
	}
	if len(client.Connections()) != 0 || len(client.PullRequestSources()) != 0 || len(client.IdentityResolvers()) != 0 {
		t.Errorf("client = %+v, want all accessors empty", client)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// TestClient_ConcurrentReadsAreSafe backs up Client's doc comment: a *Client
// is immutable after construction, so concurrent reads from multiple
// goroutines are safe. Meaningful under `go test -race`.
func TestClient_ConcurrentReadsAreSafe(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "github-personal", Type: "github", Auth: config.AuthConfig{Token: "ghp_test"}},
			{Name: "gitlab-work", Type: "gitlab", Auth: config.AuthConfig{Token: "glpat_test"}},
		},
	}
	client, err := New(prsmConfig)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	var waitGroup sync.WaitGroup
	for range 50 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_ = client.Connections()
			_ = client.PullRequestSources()
			_ = client.IdentityResolvers()
			_ = client.FailedProviders()
			_, _ = client.PullRequestSourceFor(model.ProviderInstance{Name: "github-personal"})
		}()
	}
	waitGroup.Wait()
}
