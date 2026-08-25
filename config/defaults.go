package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// defaultConfigContent is written to the config file on first run.
const defaultConfigContent = `# prsm configuration
# Location: ~/.config/prsm/config.toml
#
# Tokens may be written inline or as environment variable references:
#   token = "ghp_actual_token"   # inline — keep this file chmod 0600
#   token = "$GITHUB_TOKEN"      # env var reference (expanded at startup)

[defaults]
refresh_interval_seconds = 60
# default_view = "my-reviews"

# ---------------------------------------------------------------------------
# Provider instances
# ---------------------------------------------------------------------------

# Token / OAuth auth (GitHub, GitLab, Gitea — recommended):
# [[providers]]
# name = "github"
# type = "github"   # "github" | "gitlab" | "gitea"
#
# # Bound how long one paginated fetch (PR list, CI, reviews) may run before
# # it returns partial results plus an error. Defaults to 30 if omitted; raise
# # it for a self-hosted instance known to be slow.
# pagination_timeout_seconds = 30
#
# [providers.auth]
# type  = "pat"     # "pat" | "oauth" | "basic"
# token = "$GITHUB_TOKEN"
#
# # Limit polling to specific repos (omit to poll all accessible repos):
# [[providers.repos]]
# owner = "my-org"
# repo  = "my-repo"

# Basic auth (Gitea only — use PAT when available; basic auth breaks with 2FA):
# [[providers]]
# name     = "my-gitea"
# type     = "gitea"
# base_url = "https://gitea.example.com"
#
# [providers.auth]
# type     = "basic"
# username = "myuser"
# password = "$GITEA_PASSWORD"

# ---------------------------------------------------------------------------
# View definitions
# ---------------------------------------------------------------------------

# [[views]]
# name     = "my-reviews"
# resource = "pr"   # required — "pr" | "issue"
#
# [views.filter]
# reviewer = "me"
# draft    = false
#
# [views.sort]
# by        = "updated"
# direction = "desc"
#
# [views.group]
# by = "repo"
`

// DefaultConfigPath returns the XDG config path for prsm.
// It respects $XDG_CONFIG_HOME and falls back to ~/.config on all platforms.
func DefaultConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(homeDirectory, ".config")
	}
	return filepath.Join(configHome, "prsm", "config.toml")
}

// CreateDefault writes a starter config scaffold to path (or DefaultConfigPath
// if path is empty). It creates the parent directory with mode 0700 and the
// file itself with mode 0600. It returns an error if the file already exists.
func CreateDefault(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("config: create directory %s: %w", directory, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("config: create %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.WriteString(defaultConfigContent); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
