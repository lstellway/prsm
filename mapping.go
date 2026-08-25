package prsm

import (
	"time"

	adaptergitea "github.com/lstellway/prsm/adapter/gitea"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	adaptergitlab "github.com/lstellway/prsm/adapter/gitlab"
	"github.com/lstellway/prsm/config"
)

// This is the assembly layer's single point of translation from config types to
// adapter types. The config layer's RepoRef and each vendor's
// own scope type are deliberately distinct so the adapter packages stay free of
// any config import; the conversion cost of that separation is paid here and
// nowhere else.
//
// There is no shared adapter-side repo reference to convert into. Scope is
// vendor vocabulary — owner/repo pairs, project and group paths, job paths, a
// filesystem root — so the conversion is per vendor and the destination type
// belongs to the vendor package.

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
		Name:              providerConfig.Name,
		Token:             providerConfig.Auth.Token,
		BaseURL:           providerConfig.BaseURL,
		Repos:             toScopeRefs(providerConfig.Repos, githubRepoRef),
		PaginationTimeout: paginationTimeout(providerConfig.PaginationTimeoutSeconds),
	}
}

// paginationTimeout converts a config seconds value into a time.Duration,
// leaving it at zero when unset so the adapter applies its own default —
// config.LoadFile already rejects negative values before this is called.
func paginationTimeout(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

// gitlabConfig maps a config.ProviderConfig to the GitLab adapter's Config type.
// Basic-auth credentials have no GitLab equivalent and are dropped.
// PaginationTimeoutSeconds has no equivalent yet either — the GitLab adapter
// is not implemented, so there is nowhere to carry it — and is dropped the
// same way until that adapter's own Config gains the field.
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
// PaginationTimeoutSeconds has no equivalent yet either — the Gitea adapter
// is not implemented, so there is nowhere to carry it — and is dropped the
// same way until that adapter's own Config gains the field.
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
