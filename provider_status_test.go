package prsm

import (
	"errors"
	"testing"

	"github.com/lstellway/prsm/model"
)

func TestProviderStatuses_MergesConnectedAndConstructFailed(t *testing.T) {
	snapshot := PullRequestSnapshot{
		Connections: []ConnectionStatus{
			{
				Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub},
				State:    ConnectionStateOK,
			},
			{
				Provider: model.ProviderInstance{Name: "github-work", Kind: model.ProviderGitHub},
				State:    ConnectionStateOffline,
				Err:      errors.New("dial tcp: i/o timeout"),
			},
		},
	}
	client := &Client{
		failedProviders: []*ConstructError{
			{Provider: "gitlab-work", Kind: model.ProviderGitLab, Reason: ConstructErrorReasonNotImplemented},
		},
	}

	statuses := client.ProviderStatuses(snapshot)

	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3: %+v", len(statuses), statuses)
	}

	if got := statuses[0]; got.Provider != "github-personal" || got.Kind != model.ProviderGitHub ||
		got.Phase != ProviderPhaseConnected || got.Label() != "ok" || got.Err != nil {
		t.Errorf("statuses[0] = %+v", got)
	}
	if got := statuses[1]; got.Provider != "github-work" || got.Phase != ProviderPhaseConnected ||
		got.Label() != "offline" || got.Err == nil || got.Err.Error() != "dial tcp: i/o timeout" {
		t.Errorf("statuses[1] = %+v", got)
	}
	if got := statuses[2]; got.Provider != "gitlab-work" || got.Kind != model.ProviderGitLab ||
		got.Phase != ProviderPhaseConstructFailed || got.Label() != "not_implemented" || got.Err == nil {
		t.Errorf("statuses[2] = %+v", got)
	}

	// The ProviderPhaseConstructFailed Err is the *ConstructError itself, so
	// its Error() message matches ConstructError.Error()'s formatted text,
	// not just the bare reason.
	if got := statuses[2].Err.Error(); got != `construct provider "gitlab-work": "gitlab" adapter is not implemented yet` {
		t.Errorf("statuses[2].Err.Error() = %q", got)
	}
}

func TestProviderStatuses_Empty(t *testing.T) {
	client := &Client{}

	statuses := client.ProviderStatuses(PullRequestSnapshot{})

	if len(statuses) != 0 {
		t.Errorf("got %d statuses, want 0: %+v", len(statuses), statuses)
	}
}

func TestProviderStatus_Label_UnknownPhase(t *testing.T) {
	var providerStatus ProviderStatus
	if got := providerStatus.Label(); got != "unknown" {
		t.Errorf("Label() = %q, want %q for a zero-value ProviderStatus", got, "unknown")
	}
}
