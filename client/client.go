package client

import (
	"fmt"

	"github.com/lstellway/prsm/adapter"
	adaptergitea "github.com/lstellway/prsm/adapter/gitea"
	adaptergithub "github.com/lstellway/prsm/adapter/github"
	adaptergitlab "github.com/lstellway/prsm/adapter/gitlab"
	"github.com/lstellway/prsm/config"
)

// githubConfig maps a config.ProviderConfig to the GitHub adapter's Config type.
func githubConfig(pc config.ProviderConfig) adaptergithub.Config {
	repos := make([]adapter.RepoRef, len(pc.Repos))
	for i, r := range pc.Repos {
		repos[i] = adapter.RepoRef{Owner: r.Owner, Repo: r.Repo}
	}
	return adaptergithub.Config{
		Name:    pc.Name,
		Token:   pc.Auth.Token,
		BaseURL: pc.BaseURL,
		Repos:   repos,
	}
}

// gitlabConfig maps a config.ProviderConfig to the GitLab adapter's Config type.
func gitlabConfig(pc config.ProviderConfig) adaptergitlab.Config {
	repos := make([]adapter.RepoRef, len(pc.Repos))
	for i, r := range pc.Repos {
		repos[i] = adapter.RepoRef{Owner: r.Owner, Repo: r.Repo}
	}
	groups := make([]adapter.GroupRef, len(pc.Groups))
	for i, g := range pc.Groups {
		groups[i] = adapter.GroupRef{Path: g.Path}
	}
	return adaptergitlab.Config{
		Name:    pc.Name,
		Token:   pc.Auth.Token,
		BaseURL: pc.BaseURL,
		Repos:   repos,
		Groups:  groups,
	}
}

// giteaConfig maps a config.ProviderConfig to the Gitea adapter's Config type.
func giteaConfig(pc config.ProviderConfig) adaptergitea.Config {
	repos := make([]adapter.RepoRef, len(pc.Repos))
	for i, r := range pc.Repos {
		repos[i] = adapter.RepoRef{Owner: r.Owner, Repo: r.Repo}
	}
	return adaptergitea.Config{
		Name:     pc.Name,
		Token:    pc.Auth.Token,
		BaseURL:  pc.BaseURL,
		Repos:    repos,
		Username: pc.Auth.Username,
		Password: pc.Auth.Password,
	}
}

// providerKindFor returns the normalized provider kind string for pc.Type,
// or an error if the type is unrecognized.
func providerKindFor(pc config.ProviderConfig) (string, error) {
	switch pc.Type {
	case "github":
		return "github", nil
	case "gitlab":
		return "gitlab", nil
	case "gitea":
		return "gitea", nil
	default:
		return "", fmt.Errorf("unknown provider type %q for provider %q", pc.Type, pc.Name)
	}
}
