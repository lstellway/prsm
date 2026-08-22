package client

import (
	adaptergitea "github.com/lstellway/prsm/adapter/gitea"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	adaptergitlab "github.com/lstellway/prsm/adapter/gitlab"
	"github.com/lstellway/prsm/config"
)

// This is the assembly layer's single point of translation from config types to
// adapter types (STE-68, ADR-008). The config layer's RepoRef and each vendor's
// own scope type are deliberately distinct so the adapter packages stay free of
// any config import; the conversion cost of that separation is paid here and
// nowhere else.
//
// There is no shared adapter-side repo reference to convert into. Scope is
// vendor vocabulary — owner/repo pairs, project and group paths, job paths, a
// filesystem root — so the conversion is per vendor and the destination type
// belongs to the vendor package (STE-76, amending ADR-008 §5).

// toScopeRefs converts config repo references into a vendor's own scope type,
// preserving order. A nil input yields an empty, non-nil slice.
func toScopeRefs[ScopeRef any](
	repoRefs []config.RepoRef, convert func(config.RepoRef) ScopeRef,
) []ScopeRef {
	converted := make([]ScopeRef, len(repoRefs))
	for index, repoRef := range repoRefs {
		converted[index] = convert(repoRef)
	}
	return converted
}

func githubRepoRef(repoRef config.RepoRef) adaptergithub.RepoRef {
	return adaptergithub.RepoRef{Owner: repoRef.Owner, Repo: repoRef.Repo}
}

func gitlabProjectRef(repoRef config.RepoRef) adaptergitlab.ProjectRef {
	return adaptergitlab.ProjectRef{Owner: repoRef.Owner, Repo: repoRef.Repo}
}

func giteaRepoRef(repoRef config.RepoRef) adaptergitea.RepoRef {
	return adaptergitea.RepoRef{Owner: repoRef.Owner, Repo: repoRef.Repo}
}

// toGroupRefs converts config group references into their GitLab adapter
// equivalents. Groups are a GitLab-only concept, so this has one caller.
func toGroupRefs(groupRefs []config.GroupRef) []adaptergitlab.GroupRef {
	converted := make([]adaptergitlab.GroupRef, len(groupRefs))
	for index, groupRef := range groupRefs {
		converted[index] = adaptergitlab.GroupRef{Path: groupRef.Path}
	}
	return converted
}

// githubConfig maps a config.ProviderConfig to the GitHub adapter's Config type.
// Groups and basic-auth credentials have no GitHub equivalent and are dropped.
func githubConfig(providerConfig config.ProviderConfig) adaptergithub.Config {
	return adaptergithub.Config{
		Name:    providerConfig.Name,
		Token:   providerConfig.Auth.Token,
		BaseURL: providerConfig.BaseURL,
		Repos:   toScopeRefs(providerConfig.Repos, githubRepoRef),
	}
}

// gitlabConfig maps a config.ProviderConfig to the GitLab adapter's Config type.
// Basic-auth credentials have no GitLab equivalent and are dropped.
func gitlabConfig(providerConfig config.ProviderConfig) adaptergitlab.Config {
	return adaptergitlab.Config{
		Name:     providerConfig.Name,
		Token:    providerConfig.Auth.Token,
		BaseURL:  providerConfig.BaseURL,
		Projects: toScopeRefs(providerConfig.Repos, gitlabProjectRef),
		Groups:   toGroupRefs(providerConfig.Groups),
	}
}

// giteaConfig maps a config.ProviderConfig to the Gitea adapter's Config type.
// Gitea is the only provider accepting basic auth, so Username/Password are
// carried through; groups have no Gitea equivalent and are dropped.
func giteaConfig(providerConfig config.ProviderConfig) adaptergitea.Config {
	return adaptergitea.Config{
		Name:     providerConfig.Name,
		Token:    providerConfig.Auth.Token,
		BaseURL:  providerConfig.BaseURL,
		Repos:    toScopeRefs(providerConfig.Repos, giteaRepoRef),
		Username: providerConfig.Auth.Username,
		Password: providerConfig.Auth.Password,
	}
}
