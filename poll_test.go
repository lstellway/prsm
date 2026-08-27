package prsm

import (
	"context"
	"testing"
	"time"

	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
)

func TestClientPoll_StartupBurst(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub}},
		PullRequests: []model.PullRequest{{Number: 1}},
	}
	client := NewWithConnections(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A long Interval means a fast first update can only be the startup
	// burst, never the ticker.
	poller := client.Poll(ctx, PollOptions{Interval: time.Hour})

	select {
	case snapshot := <-poller.Updates():
		if len(snapshot.PullRequests) != 1 || snapshot.PullRequests[0].Number != 1 {
			t.Errorf("PullRequests = %+v, want one PR numbered 1", snapshot.PullRequests)
		}
		if len(snapshot.Connections) != 1 || snapshot.Connections[0].Provider.Name != "github-personal" {
			t.Errorf("Connections = %+v, want one status for github-personal", snapshot.Connections)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no update received within 2s of the startup burst")
	}
}

func TestClientPoll_UpdatesClosesOnContextCancel(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "github-personal"}},
	}
	client := NewWithConnections(source)

	ctx, cancel := context.WithCancel(context.Background())
	poller := client.Poll(ctx, PollOptions{Interval: time.Hour})

	<-poller.Updates() // let the startup burst land first
	cancel()

	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-poller.Updates():
			if !ok {
				return // closed, as expected
			}
		case <-deadline:
			t.Fatal("Updates() did not close within 1s of ctx cancellation")
		}
	}
}

func TestClientPoll_CurrentBeforeAnyReport(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "slow"}},
		Delay:      2 * time.Second, // outlives this test's own timeout below
	}
	client := NewWithConnections(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller := client.Poll(ctx, PollOptions{Interval: time.Hour})

	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	snapshot := poller.Current(queryCtx)

	if len(snapshot.PullRequests) != 0 {
		t.Errorf("PullRequests = %v, want none before the first fetch has completed", snapshot.PullRequests)
	}
	if len(snapshot.Connections) != 0 {
		t.Errorf("Connections = %v, want none: the engine's state is the zero value until it processes its first report", snapshot.Connections)
	}
}

func TestClientPoll_CurrentReturnsLatestKnownState(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection:   mock.Connection{InstanceVal: model.ProviderInstance{Name: "github-personal"}},
		PullRequests: []model.PullRequest{{Number: 42}},
	}
	client := NewWithConnections(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poller := client.Poll(ctx, PollOptions{Interval: time.Hour})
	<-poller.Updates() // wait for the startup burst so Current has something fresh to report

	snapshot := poller.Current(ctx)
	if len(snapshot.PullRequests) != 1 || snapshot.PullRequests[0].Number != 42 {
		t.Errorf("PullRequests = %+v, want one PR numbered 42", snapshot.PullRequests)
	}
}

func TestClientPoll_CurrentAfterStopReturnsZeroWithoutHanging(t *testing.T) {
	source := &mock.PullRequestSource{
		Connection: mock.Connection{InstanceVal: model.ProviderInstance{Name: "github-personal"}},
	}
	client := NewWithConnections(source)

	ctx, cancel := context.WithCancel(context.Background())
	poller := client.Poll(ctx, PollOptions{Interval: time.Hour})
	<-poller.Updates() // let the startup burst land, then stop the poller
	cancel()

	result := make(chan PollSnapshot, 1)
	go func() {
		// context.Background() has no deadline: without the poller's own
		// internal done-channel guard, this would hang forever once the
		// poller has already stopped and nothing is left to answer it.
		result <- poller.Current(context.Background())
	}()

	select {
	case snapshot := <-result:
		if len(snapshot.PullRequests) != 0 || len(snapshot.Connections) != 0 {
			t.Errorf("Current() after stop = %+v, want the zero PollSnapshot", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("Current() with a deadline-less ctx hung after the poller stopped")
	}
}

func TestToPollSnapshot_MapsFields(t *testing.T) {
	asOf := time.Now()
	generic := engineSnapshot[model.PullRequest]{
		Items: []model.PullRequest{{Number: 7}},
		Connections: []PolledConnectionStatus{
			{ConnectionStatus: ConnectionStatus{connectionOutcome: connectionOutcome{
				Provider: model.ProviderInstance{Name: "x"}, State: ConnectionStateOK,
			}}},
		},
		AsOf: asOf,
	}

	snapshot := toPollSnapshot(generic)

	if len(snapshot.PullRequests) != 1 || snapshot.PullRequests[0].Number != 7 {
		t.Errorf("PullRequests = %+v, want one PR numbered 7", snapshot.PullRequests)
	}
	if len(snapshot.Connections) != 1 || snapshot.Connections[0].Provider.Name != "x" {
		t.Errorf("Connections = %+v, want one status for provider x", snapshot.Connections)
	}
	if !snapshot.AsOf.Equal(asOf) {
		t.Errorf("AsOf = %v, want %v", snapshot.AsOf, asOf)
	}
}
