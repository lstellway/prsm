package prsm

import (
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
}

func TestNew_NilConfig(t *testing.T) {
	client, err := New(nil)
	if err == nil {
		t.Fatal("New(nil) error = nil, want a non-nil error")
	}
	if client == nil {
		t.Fatal("New(nil) client = nil, want a non-nil Client")
	}
	if len(client.Connections()) != 0 {
		t.Errorf("Connections() = %d, want 0", len(client.Connections()))
	}
}

// ---------------------------------------------------------------------------
// NewWithAdapters
// ---------------------------------------------------------------------------

func TestNewWithAdapters_CapabilityIndexing(t *testing.T) {
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

	client := NewWithAdapters(neither, pullRequestOnly, identityOnly, both)

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
}

func TestNewWithAdapters_Empty(t *testing.T) {
	client := NewWithAdapters()
	if client == nil {
		t.Fatal("NewWithAdapters() = nil, want a non-nil Client")
	}
	if len(client.Connections()) != 0 || len(client.PullRequestSources()) != 0 || len(client.IdentityResolvers()) != 0 {
		t.Errorf("client = %+v, want all accessors empty", client)
	}
}
