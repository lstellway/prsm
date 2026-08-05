package client

import (
	"reflect"
	"testing"

	"github.com/lstellway/prsm/adapter"
	adaptergitea "github.com/lstellway/prsm/adapter/gitea"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	adaptergitlab "github.com/lstellway/prsm/adapter/gitlab"
	"github.com/lstellway/prsm/config"
)

// These tests pin the config.ProviderConfig -> adapter Config mapping that keeps
// the adapter packages free of any config import (STE-68). They are the only
// place the field-by-field correspondence is asserted, so a field added to
// config.ProviderConfig and forgotten here shows up as a mapping gap.

// ---------------------------------------------------------------------------
// githubConfig
// ---------------------------------------------------------------------------

func TestGitHubConfig(t *testing.T) {
	cases := []struct {
		name string
		in   config.ProviderConfig
		want adaptergithub.Config
	}{
		{
			name: "full",
			in: config.ProviderConfig{
				Name:    "github-personal",
				Type:    "github",
				BaseURL: "https://ghe.example.com/api/v3",
				Auth:    config.AuthConfig{Type: "pat", Token: "ghp_token"},
				Repos: []config.RepoRef{
					{Owner: "acme-corp", Repo: "api-service"},
					{Owner: "acme-corp", Repo: "frontend"},
				},
			},
			want: adaptergithub.Config{
				Name:    "github-personal",
				Token:   "ghp_token",
				BaseURL: "https://ghe.example.com/api/v3",
				Repos: []adapter.RepoRef{
					{Owner: "acme-corp", Repo: "api-service"},
					{Owner: "acme-corp", Repo: "frontend"},
				},
			},
		},
		{
			name: "no_repos_yields_empty_non_nil_slice",
			in: config.ProviderConfig{
				Name: "github-personal",
				Auth: config.AuthConfig{Token: "ghp_token"},
			},
			want: adaptergithub.Config{
				Name:  "github-personal",
				Token: "ghp_token",
				Repos: []adapter.RepoRef{},
			},
		},
		{
			// GitHub has no group concept: groups configured on a github
			// provider are dropped rather than silently misapplied.
			name: "groups_are_dropped",
			in: config.ProviderConfig{
				Name:   "github-personal",
				Auth:   config.AuthConfig{Token: "ghp_token"},
				Groups: []config.GroupRef{{Path: "platform-team"}},
			},
			want: adaptergithub.Config{
				Name:  "github-personal",
				Token: "ghp_token",
				Repos: []adapter.RepoRef{},
			},
		},
		{
			// Basic-auth credentials are Gitea-only; the GitHub adapter has no
			// field for them and must not carry them forward.
			name: "basic_auth_credentials_are_dropped",
			in: config.ProviderConfig{
				Name: "github-personal",
				Auth: config.AuthConfig{
					Type:     "basic",
					Token:    "ghp_token",
					Username: "alice",
					Password: "hunter2",
				},
			},
			want: adaptergithub.Config{
				Name:  "github-personal",
				Token: "ghp_token",
				Repos: []adapter.RepoRef{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := githubConfig(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("githubConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// gitlabConfig
// ---------------------------------------------------------------------------

func TestGitLabConfig(t *testing.T) {
	cases := []struct {
		name string
		in   config.ProviderConfig
		want adaptergitlab.Config
	}{
		{
			name: "repos_and_groups",
			in: config.ProviderConfig{
				Name:    "gitlab-work",
				Type:    "gitlab",
				BaseURL: "https://gitlab.internal.example.com",
				Auth:    config.AuthConfig{Type: "pat", Token: "glpat_token"},
				Repos:   []config.RepoRef{{Owner: "platform", Repo: "gateway"}},
				Groups: []config.GroupRef{
					{Path: "platform-team"},
					{Path: "infra"},
				},
			},
			want: adaptergitlab.Config{
				Name:    "gitlab-work",
				Token:   "glpat_token",
				BaseURL: "https://gitlab.internal.example.com",
				Repos:   []adapter.RepoRef{{Owner: "platform", Repo: "gateway"}},
				Groups: []adaptergitlab.GroupRef{
					{Path: "platform-team"},
					{Path: "infra"},
				},
			},
		},
		{
			name: "groups_only",
			in: config.ProviderConfig{
				Name:   "gitlab-work",
				Auth:   config.AuthConfig{Token: "glpat_token"},
				Groups: []config.GroupRef{{Path: "platform-team"}},
			},
			want: adaptergitlab.Config{
				Name:   "gitlab-work",
				Token:  "glpat_token",
				Repos:  []adapter.RepoRef{},
				Groups: []adaptergitlab.GroupRef{{Path: "platform-team"}},
			},
		},
		{
			name: "neither_repos_nor_groups",
			in: config.ProviderConfig{
				Name: "gitlab-work",
				Auth: config.AuthConfig{Token: "glpat_token"},
			},
			want: adaptergitlab.Config{
				Name:   "gitlab-work",
				Token:  "glpat_token",
				Repos:  []adapter.RepoRef{},
				Groups: []adaptergitlab.GroupRef{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gitlabConfig(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("gitlabConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// giteaConfig
// ---------------------------------------------------------------------------

func TestGiteaConfig(t *testing.T) {
	cases := []struct {
		name string
		in   config.ProviderConfig
		want adaptergitea.Config
	}{
		{
			name: "pat_auth",
			in: config.ProviderConfig{
				Name:    "codeberg",
				Type:    "gitea",
				BaseURL: "https://codeberg.org",
				Auth:    config.AuthConfig{Type: "pat", Token: "gitea_token"},
				Repos:   []config.RepoRef{{Owner: "loganstellway", Repo: "public-project"}},
			},
			want: adaptergitea.Config{
				Name:    "codeberg",
				Token:   "gitea_token",
				BaseURL: "https://codeberg.org",
				Repos:   []adapter.RepoRef{{Owner: "loganstellway", Repo: "public-project"}},
			},
		},
		{
			// Gitea is the only provider supporting basic auth, so these
			// fields must survive the mapping.
			name: "basic_auth_credentials_are_carried",
			in: config.ProviderConfig{
				Name:    "gitea-self-hosted",
				BaseURL: "https://gitea.example.com",
				Auth: config.AuthConfig{
					Type:     "basic",
					Username: "alice",
					Password: "hunter2",
				},
			},
			want: adaptergitea.Config{
				Name:     "gitea-self-hosted",
				BaseURL:  "https://gitea.example.com",
				Repos:    []adapter.RepoRef{},
				Username: "alice",
				Password: "hunter2",
			},
		},
		{
			// Both are mapped; the token-takes-precedence rule documented on
			// gitea.Config is the constructor's job, not the mapper's.
			name: "token_and_basic_auth_both_mapped",
			in: config.ProviderConfig{
				Name: "gitea-self-hosted",
				Auth: config.AuthConfig{
					Token:    "gitea_token",
					Username: "alice",
					Password: "hunter2",
				},
			},
			want: adaptergitea.Config{
				Name:     "gitea-self-hosted",
				Token:    "gitea_token",
				Repos:    []adapter.RepoRef{},
				Username: "alice",
				Password: "hunter2",
			},
		},
		{
			// Gitea has no group concept.
			name: "groups_are_dropped",
			in: config.ProviderConfig{
				Name:   "codeberg",
				Auth:   config.AuthConfig{Token: "gitea_token"},
				Groups: []config.GroupRef{{Path: "some-org"}},
			},
			want: adaptergitea.Config{
				Name:  "codeberg",
				Token: "gitea_token",
				Repos: []adapter.RepoRef{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := giteaConfig(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("giteaConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cross-mapper invariants
// ---------------------------------------------------------------------------

// TestRepoRefMappingPreservesOrder guards the index-assignment loops shared by
// all three mappers: repo order is meaningful for polling and must round-trip.
func TestRepoRefMappingPreservesOrder(t *testing.T) {
	in := config.ProviderConfig{
		Name: "p",
		Auth: config.AuthConfig{Token: "t"},
		Repos: []config.RepoRef{
			{Owner: "o1", Repo: "r1"},
			{Owner: "o2", Repo: "r2"},
			{Owner: "o3", Repo: "r3"},
		},
	}
	want := []adapter.RepoRef{
		{Owner: "o1", Repo: "r1"},
		{Owner: "o2", Repo: "r2"},
		{Owner: "o3", Repo: "r3"},
	}

	if got := githubConfig(in).Repos; !reflect.DeepEqual(got, want) {
		t.Errorf("githubConfig().Repos = %+v, want %+v", got, want)
	}
	if got := gitlabConfig(in).Repos; !reflect.DeepEqual(got, want) {
		t.Errorf("gitlabConfig().Repos = %+v, want %+v", got, want)
	}
	if got := giteaConfig(in).Repos; !reflect.DeepEqual(got, want) {
		t.Errorf("giteaConfig().Repos = %+v, want %+v", got, want)
	}
}

// TestMappersCarryToken asserts the one field every provider needs, across all
// three mappers, so a copy-paste slip in any single mapper is caught.
func TestMappersCarryToken(t *testing.T) {
	in := config.ProviderConfig{
		Name:    "p",
		BaseURL: "https://example.com",
		Auth:    config.AuthConfig{Type: "pat", Token: "secret-token"},
	}

	if got := githubConfig(in); got.Token != "secret-token" || got.Name != "p" || got.BaseURL != "https://example.com" {
		t.Errorf("githubConfig() = %+v, want Name/Token/BaseURL carried", got)
	}
	if got := gitlabConfig(in); got.Token != "secret-token" || got.Name != "p" || got.BaseURL != "https://example.com" {
		t.Errorf("gitlabConfig() = %+v, want Name/Token/BaseURL carried", got)
	}
	if got := giteaConfig(in); got.Token != "secret-token" || got.Name != "p" || got.BaseURL != "https://example.com" {
		t.Errorf("giteaConfig() = %+v, want Name/Token/BaseURL carried", got)
	}
}
