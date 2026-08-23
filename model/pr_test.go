package model_test

import (
	"testing"

	"github.com/lstellway/prsm/model"
)

// TestPullRequestRefCopiesIntendedFields guards Ref() against silent field
// drift: PullRequestRef is a hand-picked subset of PullRequest's fields, and
// nothing but this test enforces that Ref() actually copies each of them.
// want is built from literals rather than by re-reading pullRequest's own
// fields, so a Ref() that quietly drops or mismaps a field fails here
// instead of trivially agreeing with itself.
func TestPullRequestRefCopiesIntendedFields(t *testing.T) {
	provider := model.ProviderInstance{
		Name: "instance-name", Kind: model.ProviderGitHub, Host: "github.example.com", Account: "octocat",
	}
	repo := model.Repository{Owner: "owner-name", Name: "repo-name"}

	pullRequest := model.PullRequest{
		ProviderID: "provider-id-123",
		Number:     42,
		Provider:   provider,
		Repo:       repo,
		HeadSHA:    "deadbeef",
		Title:      "content fields must not leak into Ref",
	}

	got := pullRequest.Ref()
	want := model.PullRequestRef{
		Provider:   provider,
		Repo:       repo,
		Number:     42,
		ProviderID: "provider-id-123",
		HeadSHA:    "deadbeef",
	}

	if got != want {
		t.Errorf("Ref() = %+v, want %+v", got, want)
	}
}
