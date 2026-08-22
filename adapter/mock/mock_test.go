package mock_test

import (
	"context"
	"testing"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
)

// TestConnectionWithoutHostCredentialOrResourceKinds is STE-76's falsification
// check for the interface split. The local git checkout has no host, no
// credential, and serves neither pull requests nor an identity; if expressing
// that is awkward, the split is in the wrong place. It is three lines.
//
// The negative assertions are the point. A base-only connection must not
// accidentally satisfy the resource-kind interfaces, because assembly reads
// those assertions as "this connection serves pull requests" — an interface
// that a stub could satisfy would put us back where a fat interface left us,
// with a structural fact reported as a runtime error.
func TestConnectionWithoutHostCredentialOrResourceKinds(t *testing.T) {
	var connection adapter.Connection = &mock.Connection{
		InstanceVal: model.ProviderInstance{Name: "local-checkout"},
	}

	if got := connection.Instance().Name; got != "local-checkout" {
		t.Errorf("Instance().Name = %q, want %q", got, "local-checkout")
	}
	if got := connection.Instance().Host; got != "" {
		t.Errorf("Instance().Host = %q, want empty: a source may have no host", got)
	}

	if _, servesPullRequests := connection.(adapter.PullRequestSource); servesPullRequests {
		t.Error("base-only connection satisfies adapter.PullRequestSource; " +
			"assembly would fan a PR fetch out to a source that serves none")
	}
	if _, resolvesIdentity := connection.(adapter.IdentityResolver); resolvesIdentity {
		t.Error("base-only connection satisfies adapter.IdentityResolver; " +
			"assembly would report a credential-less source as auth-failed")
	}
}

// TestPullRequestSourceWithoutIdentity pins the middle case: a source that
// serves a resource kind but authenticates as nobody, such as an anonymous
// read-only connection to a public host.
func TestPullRequestSourceWithoutIdentity(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "anonymous"}},
		PullRequests: []model.PullRequest{{Number: 7}},
	}

	var connection adapter.Connection = source
	if _, resolvesIdentity := connection.(adapter.IdentityResolver); resolvesIdentity {
		t.Error("PullRequestSource satisfies adapter.IdentityResolver without one embedded")
	}

	pullRequests, err := source.ListPullRequests(context.Background())
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if len(pullRequests) != 1 || pullRequests[0].Number != 7 {
		t.Errorf("ListPullRequests() = %+v, want one PR numbered 7", pullRequests)
	}
}

// TestAdapterServesEveryKind confirms the composed mock still reaches both
// interfaces through a single value, which is what most tests will hold.
func TestAdapterServesEveryKind(t *testing.T) {
	var connection adapter.Connection = &mock.Adapter{
		PullRequestSource: mock.PullRequestSource{
			Connection: mock.Connection{InstanceVal: model.ProviderInstance{
				Name: "github-personal",
				Kind: model.ProviderGitHub,
				Host: "github.com",
			}},
		},
		IdentityResolver: mock.IdentityResolver{
			Identity: model.Identity{Username: "alice"},
		},
	}

	if _, servesPullRequests := connection.(adapter.PullRequestSource); !servesPullRequests {
		t.Error("Adapter does not satisfy adapter.PullRequestSource")
	}

	resolver, resolvesIdentity := connection.(adapter.IdentityResolver)
	if !resolvesIdentity {
		t.Fatal("Adapter does not satisfy adapter.IdentityResolver")
	}
	identity, err := resolver.ResolveIdentity(context.Background())
	if err != nil {
		t.Fatalf("ResolveIdentity() error = %v", err)
	}
	if identity.Username != "alice" {
		t.Errorf("ResolveIdentity() username = %q, want %q", identity.Username, "alice")
	}
}
