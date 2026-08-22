package prsm

import (
	"errors"
	"reflect"
	"strings"
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
	if err == nil {
		t.Fatal("New() error = nil, want a construction error for the gitlab provider")
	}
	if !strings.Contains(err.Error(), "gitlab-work") || !strings.Contains(err.Error(), "gitlab") {
		t.Errorf("New() error = %q, want it to name the failed gitlab provider", err.Error())
	}
	if strings.Contains(err.Error(), "github-personal") {
		t.Errorf("New() error = %q, want no mention of the successful github provider", err.Error())
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
	if err == nil {
		t.Fatal("New() error = nil, want construction errors for gitlab and gitea")
	}
	if !strings.Contains(err.Error(), "gitlab-work") {
		t.Errorf("New() error = %q, want it to name the failed gitlab provider", err.Error())
	}
	if !strings.Contains(err.Error(), "gitea-home") {
		t.Errorf("New() error = %q, want it to name the failed gitea provider", err.Error())
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

func TestNew_ErrorJoinStructure(t *testing.T) {
	prsmConfig := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "gitlab-work", Type: "gitlab"},
			{Name: "gitea-home", Type: "gitea"},
		},
	}

	_, err := New(prsmConfig)
	if err == nil {
		t.Fatal("New() error = nil, want construction errors")
	}

	// New's doc comment promises errors.Join, which callers rely on to walk
	// discrete errors via the multi-error Unwrap() []error form rather than
	// only ever reading the flattened message string.
	joinedErr, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("New() error does not implement Unwrap() []error; want an errors.Join result")
	}
	unwrapped := joinedErr.Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("len(Unwrap()) = %d, want 2 discrete errors", len(unwrapped))
	}

	for _, singleErr := range unwrapped {
		var constructError *ConstructError
		if !errors.As(singleErr, &constructError) {
			t.Errorf("error %v does not unwrap to a *ConstructError", singleErr)
		}
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
	if err == nil {
		t.Fatal("New() error = nil, want a construction error")
	}
	if !strings.Contains(err.Error(), "gitlab-work") {
		t.Errorf("New() error = %q, want it to name the provider", err.Error())
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
	if err == nil {
		t.Fatal("New() error = nil, want a construction error for an unrecognized type")
	}
	if !strings.Contains(err.Error(), "unknown provider type") || !strings.Contains(err.Error(), "bitbucket") {
		t.Errorf("New() error = %q, want it to mention the unknown type", err.Error())
	}
	if len(client.Connections()) != 0 {
		t.Errorf("Connections() = %d, want 0", len(client.Connections()))
	}

	var constructError *ConstructError
	if !errors.As(err, &constructError) {
		t.Fatal("errors.As(err, &ConstructError{}) = false, want true")
	}
	if constructError.Reason != ConstructErrorReasonUnknownType {
		t.Errorf("Reason = %v, want ConstructErrorReasonUnknownType", constructError.Reason)
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
	if err == nil {
		t.Fatal("New() error = nil, want a construction error for the duplicate name")
	}

	var constructError *ConstructError
	if !errors.As(err, &constructError) {
		t.Fatal("errors.As(err, &ConstructError{}) = false, want true")
	}
	if constructError.Reason != ConstructErrorReasonDuplicateName {
		t.Errorf("Reason = %v, want ConstructErrorReasonDuplicateName", constructError.Reason)
	}

	// The first occurrence still constructs; only the repeat is rejected.
	if len(client.Connections()) != 1 {
		t.Fatalf("Connections() = %d, want 1", len(client.Connections()))
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

func TestNewWithConnections_AccessorsReturnCopies(t *testing.T) {
	connection := &mock.Connection{InstanceVal: model.ProviderInstance{Name: "original"}}
	client := NewWithConnections(connection)

	connections := client.Connections()
	connections[0] = &mock.Connection{InstanceVal: model.ProviderInstance{Name: "mutated"}}

	if got := client.Connections()[0].Instance().Name; got != "original" {
		t.Errorf("Client.connections[0].Instance().Name = %q after mutating a returned slice, want %q (accessor must return a copy)", got, "original")
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
