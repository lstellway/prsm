package prsm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func identityStatusFor(statuses []IdentityStatus, name string) (IdentityStatus, bool) {
	for _, status := range statuses {
		if status.Provider.Name == name {
			return status, true
		}
	}
	return IdentityStatus{}, false
}

// newMockIdentityOnly builds a single named GitHub connection that resolves
// identity or err but does not serve pull requests, cutting the
// identityOnlyConnection{Connection: ..., IdentityResolver: ...} literal
// every ResolveIdentities test would otherwise repeat.
func newMockIdentityOnly(name string, identity model.Identity, err error) *identityOnlyConnection {
	return &identityOnlyConnection{
		Connection:       mock.Connection{InstanceVal: model.ProviderInstance{Name: name, Kind: model.ProviderGitHub}},
		IdentityResolver: mock.IdentityResolver{Identity: identity, IdentityErr: err},
	}
}

func TestNewIdentityStatus_Success(t *testing.T) {
	instance := model.ProviderInstance{Name: "instance", Kind: model.ProviderGitHub}
	resolvedAt := time.Now()

	status := newIdentityStatus(instance, resolvedAt, nil)

	if status.Provider != instance {
		t.Errorf("Provider = %+v, want %+v", status.Provider, instance)
	}
	if status.State != ConnectionStateOK {
		t.Errorf("State = %v, want ConnectionStateOK", status.State)
	}
	if !status.ResolvedAt.Equal(resolvedAt) {
		t.Errorf("ResolvedAt = %v, want %v", status.ResolvedAt, resolvedAt)
	}
	if status.Err != nil {
		t.Errorf("Err = %v, want nil", status.Err)
	}
}

// TestNewIdentityStatus_Failure only exercises one failure case: the full
// err-to-state taxonomy (rate limit, auth, joined errors, ...) is already
// covered by TestNewConnectionStatus_Classification against the
// classifyConnectionState helper newIdentityStatus shares with
// newConnectionStatus. This test only pins down that newIdentityStatus wires
// that classification through and leaves ResolvedAt zero on failure.
func TestNewIdentityStatus_Failure(t *testing.T) {
	sentinelTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	authErr := adapter.AuthError{}

	status := newIdentityStatus(model.ProviderInstance{Name: "instance"}, sentinelTime, authErr)

	if status.State != ConnectionStateUnauthorized {
		t.Errorf("State = %v, want ConnectionStateUnauthorized", status.State)
	}
	if status.Err != authErr {
		t.Errorf("Err = %v, want %v", status.Err, authErr)
	}
	if !status.ResolvedAt.IsZero() {
		t.Errorf("ResolvedAt = %v, want zero", status.ResolvedAt)
	}
}

func TestResolveIdentities_AllResolved(t *testing.T) {
	alice := newMockIdentityOnly("alice-instance", model.Identity{Username: "alice"}, nil)
	bob := newMockIdentityOnly("bob-instance", model.Identity{Username: "bob"}, nil)

	client := NewWithConnections(alice, bob)

	before := time.Now()
	resolvedIdentities, statuses := client.ResolveIdentities(context.Background())
	after := time.Now()

	if len(resolvedIdentities) != 2 {
		t.Fatalf("resolvedIdentities = %d entries, want 2", len(resolvedIdentities))
	}
	if got := resolvedIdentities["alice-instance"].Username; got != "alice" {
		t.Errorf("resolvedIdentities[alice-instance].Username = %q, want %q", got, "alice")
	}
	if got := resolvedIdentities["bob-instance"].Username; got != "bob" {
		t.Errorf("resolvedIdentities[bob-instance].Username = %q, want %q", got, "bob")
	}

	if len(statuses) != 2 {
		t.Fatalf("statuses = %d, want 2", len(statuses))
	}
	for _, name := range []string{"alice-instance", "bob-instance"} {
		status, ok := identityStatusFor(statuses, name)
		if !ok {
			t.Fatalf("no IdentityStatus for %q", name)
		}
		if status.State != ConnectionStateOK {
			t.Errorf("%s: State = %v, want ConnectionStateOK", name, status.State)
		}
		if status.Err != nil {
			t.Errorf("%s: Err = %v, want nil", name, status.Err)
		}
		if status.ResolvedAt.Before(before) || status.ResolvedAt.After(after) {
			t.Errorf("%s: ResolvedAt = %v, want between %v and %v", name, status.ResolvedAt, before, after)
		}
	}
}

// TestResolveIdentities_OneResolvedOneFailed is the issue's acceptance
// criterion for state 2: a connection that implements adapter.IdentityResolver
// but fails to resolve (bad credential) is reported as degraded, and
// contributes no "me" entry, while a healthy connection alongside it still
// resolves normally.
func TestResolveIdentities_OneResolvedOneFailed(t *testing.T) {
	healthy := newMockIdentityOnly("healthy", model.Identity{Username: "alice"}, nil)
	authErr := adapter.AuthError{}
	broken := newMockIdentityOnly("broken", model.Identity{}, authErr)

	client := NewWithConnections(healthy, broken)
	resolvedIdentities, statuses := client.ResolveIdentities(context.Background())

	if len(resolvedIdentities) != 1 {
		t.Fatalf("resolvedIdentities = %d entries, want 1", len(resolvedIdentities))
	}
	if _, ok := resolvedIdentities["broken"]; ok {
		t.Error("resolvedIdentities contains an entry for broken, want none")
	}
	if got := resolvedIdentities["healthy"].Username; got != "alice" {
		t.Errorf("resolvedIdentities[healthy].Username = %q, want %q", got, "alice")
	}

	healthyStatus, ok := identityStatusFor(statuses, "healthy")
	if !ok {
		t.Fatal("no IdentityStatus for healthy")
	}
	if healthyStatus.State != ConnectionStateOK {
		t.Errorf("healthy: State = %v, want ConnectionStateOK", healthyStatus.State)
	}

	brokenStatus, ok := identityStatusFor(statuses, "broken")
	if !ok {
		t.Fatal("no IdentityStatus for broken")
	}
	if brokenStatus.State != ConnectionStateUnauthorized {
		t.Errorf("broken: State = %v, want ConnectionStateUnauthorized", brokenStatus.State)
	}
	if !errors.Is(brokenStatus.Err, authErr) {
		t.Errorf("broken: Err = %v, want it to wrap %v", brokenStatus.Err, authErr)
	}
	if !brokenStatus.ResolvedAt.IsZero() {
		t.Errorf("broken: ResolvedAt = %v, want zero", brokenStatus.ResolvedAt)
	}
}

// TestResolveIdentities_NoIdentityResolvers is the issue's acceptance
// criterion for state 1: a connection that does not implement
// adapter.IdentityResolver at all — the local-checkout case, stood in for
// here by a PR-only connection — contributes no "me" entry and, critically,
// no IdentityStatus either. Reporting it would render a perfectly healthy
// credential-less source as permanently degraded.
func TestResolveIdentities_NoIdentityResolvers(t *testing.T) {
	pullRequestOnly := newMockPullRequestSource("pr-only", nil, nil)

	client := NewWithConnections(pullRequestOnly)
	resolvedIdentities, statuses := client.ResolveIdentities(context.Background())

	if len(resolvedIdentities) != 0 {
		t.Errorf("resolvedIdentities = %d entries, want 0", len(resolvedIdentities))
	}
	if len(statuses) != 0 {
		t.Errorf("statuses = %d, want 0: a connection with no identity resolver must not report as broken", len(statuses))
	}
}

func TestResolveIdentities_NoConnectionsAtAll(t *testing.T) {
	resolvedIdentities, statuses := NewWithConnections().ResolveIdentities(context.Background())

	if len(resolvedIdentities) != 0 {
		t.Errorf("resolvedIdentities = %d entries, want 0", len(resolvedIdentities))
	}
	if len(statuses) != 0 {
		t.Errorf("statuses = %d, want 0", len(statuses))
	}
}

// TestResolveIdentities_RunsConcurrently proves ResolveIdentities actually
// fans out in parallel, the same way TestFetch_RunsConnectionsConcurrently
// does for Fetch. Three connections at 100ms each would take ~300ms run
// serially; run concurrently they take ~100ms.
func TestResolveIdentities_RunsConcurrently(t *testing.T) {
	const delay = 100 * time.Millisecond

	connections := make([]adapter.Connection, 3)
	for index, name := range []string{"one", "two", "three"} {
		connections[index] = &identityOnlyConnection{
			Connection:       mock.Connection{InstanceVal: model.ProviderInstance{Name: name}},
			IdentityResolver: mock.IdentityResolver{Delay: delay},
		}
	}

	client := NewWithConnections(connections...)

	started := time.Now()
	client.ResolveIdentities(context.Background())
	elapsed := time.Since(started)

	const bound = 250 * time.Millisecond
	if elapsed >= bound {
		t.Errorf("ResolveIdentities took %v across 3 connections at %v delay each, want under %v — looks sequential, not concurrent", elapsed, delay, bound)
	}
}

// TestResolveIdentities_TwoInstancesOfOneKind is the assembly-layer half of
// the scenario PR #5 fixed on the query side: two GitHub instances, each with
// its own login, must resolve to two distinct map entries rather than one
// collapsing onto the other.
func TestResolveIdentities_TwoInstancesOfOneKind(t *testing.T) {
	personal := newMockIdentityOnly("github-personal", model.Identity{Username: "alice"}, nil)
	enterprise := newMockIdentityOnly("github-enterprise", model.Identity{Username: "alice-corp"}, nil)

	resolvedIdentities, _ := NewWithConnections(personal, enterprise).ResolveIdentities(context.Background())

	if got := resolvedIdentities["github-personal"].Username; got != "alice" {
		t.Errorf("resolvedIdentities[github-personal].Username = %q, want %q", got, "alice")
	}
	if got := resolvedIdentities["github-enterprise"].Username; got != "alice-corp" {
		t.Errorf("resolvedIdentities[github-enterprise].Username = %q, want %q", got, "alice-corp")
	}
}

// TestResolveIdentities_FeedsPRFilterCompile is the issue's overall "done
// when" criterion: author = "me" matches your own pull requests. It proves
// the map ResolveIdentities returns is usable, unmodified, as
// query.PRFilterSpec.Compile's resolvedMe argument.
func TestResolveIdentities_FeedsPRFilterCompile(t *testing.T) {
	me := newMockIdentityOnly("github-personal", model.Identity{Username: "alice"}, nil)
	resolvedIdentities, _ := NewWithConnections(me).ResolveIdentities(context.Background())

	predicate, err := (query.PRFilterSpec{BaseFilterSpec: query.BaseFilterSpec{Author: "me"}}).Compile(resolvedIdentities)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	ownPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal"},
		Author:   model.Author{Username: "alice"},
	}
	if !predicate(ownPullRequest) {
		t.Error("predicate(ownPullRequest) = false, want true: author=me should match a PR authored by the resolved identity")
	}

	someoneElsesPullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "github-personal"},
		Author:   model.Author{Username: "eve"},
	}
	if predicate(someoneElsesPullRequest) {
		t.Error("predicate(someoneElsesPullRequest) = true, want false")
	}

	// A PR from an instance whose identity never resolved (no connection for
	// it here at all) must not match "me" either, per resolveMe's contract
	// that a missing entry is a non-match, not a match against "".
	unresolvedInstancePullRequest := model.PullRequest{
		Provider: model.ProviderInstance{Name: "some-other-instance"},
		Author:   model.Author{Username: "alice"},
	}
	if predicate(unresolvedInstancePullRequest) {
		t.Error("predicate(unresolvedInstancePullRequest) = true, want false: unresolved instance must not match me")
	}
}
