package client

import (
	"github.com/lstellway/prsm/adapter"
	adaptergitea "github.com/lstellway/prsm/adapter/gitea"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	adaptergitlab "github.com/lstellway/prsm/adapter/gitlab"
	"github.com/lstellway/prsm/config"
)

// This is the assembly layer's single point of translation from config types to
// adapter types (STE-68, ADR-008). config.RepoRef and adapter.RepoRef are
// deliberately distinct so the adapter packages stay free of any config import;
// the conversion cost of that separation is paid here and nowhere else.

// toRepoRefs converts config repo references into their adapter equivalents,
// preserving order. A nil input yields an empty, non-nil slice.
func toRepoRefs(rs []config.RepoRef) []adapter.RepoRef {
	out := make([]adapter.RepoRef, len(rs))
	for i, r := range rs {
		out[i] = adapter.RepoRef{Owner: r.Owner, Repo: r.Repo}
	}
	return out
}

// toGroupRefs converts config group references into their GitLab adapter
// equivalents. Groups are a GitLab-only concept, so this has one caller.
func toGroupRefs(gs []config.GroupRef) []adaptergitlab.GroupRef {
	out := make([]adaptergitlab.GroupRef, len(gs))
	for i, g := range gs {
		out[i] = adaptergitlab.GroupRef{Path: g.Path}
	}
	return out
}

// githubConfig maps a config.ProviderConfig to the GitHub adapter's Config type.
// Groups and basic-auth credentials have no GitHub equivalent and are dropped.
func githubConfig(pc config.ProviderConfig) adaptergithub.Config {
	return adaptergithub.Config{
		Name:    pc.Name,
		Token:   pc.Auth.Token,
		BaseURL: pc.BaseURL,
		Repos:   toRepoRefs(pc.Repos),
	}
}

// gitlabConfig maps a config.ProviderConfig to the GitLab adapter's Config type.
// Basic-auth credentials have no GitLab equivalent and are dropped.
func gitlabConfig(pc config.ProviderConfig) adaptergitlab.Config {
	return adaptergitlab.Config{
		Name:    pc.Name,
		Token:   pc.Auth.Token,
		BaseURL: pc.BaseURL,
		Repos:   toRepoRefs(pc.Repos),
		Groups:  toGroupRefs(pc.Groups),
	}
}

// giteaConfig maps a config.ProviderConfig to the Gitea adapter's Config type.
// Gitea is the only provider accepting basic auth, so Username/Password are
// carried through; groups have no Gitea equivalent and are dropped.
func giteaConfig(pc config.ProviderConfig) adaptergitea.Config {
	return adaptergitea.Config{
		Name:     pc.Name,
		Token:    pc.Auth.Token,
		BaseURL:  pc.BaseURL,
		Repos:    toRepoRefs(pc.Repos),
		Username: pc.Auth.Username,
		Password: pc.Auth.Password,
	}
}
