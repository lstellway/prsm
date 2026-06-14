//go:build tools

package tools

import (
	_ "charm.land/bubbles/v2"
	_ "charm.land/bubbletea/v2"
	_ "charm.land/lipgloss/v2"
	_ "code.gitea.io/sdk/gitea"
	_ "github.com/BurntSushi/toml"
	_ "github.com/google/go-github/v88/github"
	_ "gitlab.com/gitlab-org/api/client-go"
)
