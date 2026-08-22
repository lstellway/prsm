package config

import "fmt"

// Config is the top-level structure for a prsm config file.
type Config struct {
	Defaults  Defaults         `toml:"defaults"`
	Providers []ProviderConfig `toml:"providers"`
	Views     []ViewConfig     `toml:"views"`
	Events    EventsConfig     `toml:"events"`
	Hooks     []HookConfig     `toml:"hooks"`
}

// Defaults sets session-wide starting values before any named view is applied.
type Defaults struct {
	RefreshIntervalSeconds int    `toml:"refresh_interval_seconds"`
	DefaultView            string `toml:"default_view"`
}

// ProviderConfig is one connection to a git hosting service.
type ProviderConfig struct {
	Name    string     `toml:"name"`
	Type    string     `toml:"type"` // "github" | "gitlab" | "gitea"
	BaseURL string     `toml:"base_url"`
	Auth    AuthConfig `toml:"auth"`
	Repos   []RepoRef  `toml:"repos"`
	Groups  []GroupRef `toml:"groups"`
}

// AuthConfig holds credentials for a provider.
//
// For pat and oauth: set Token. Both auth types send the same Authorization:
// Bearer header — the distinction is user-facing documentation only.
// For basic (Gitea only): set Username and Password. Note that basic auth
// breaks when 2FA is enabled on the Gitea instance; PAT is strongly preferred.
// GitHub App auth (requires app_id + private_key) is not supported in v1.
type AuthConfig struct {
	Type     string `toml:"type"`     // "pat" | "oauth" | "basic"
	Token    string `toml:"token"`    // pat, oauth
	Username string `toml:"username"` // basic only
	Password string `toml:"password"` // basic only
}

// RepoRef identifies a repository to poll within a provider.
type RepoRef struct {
	Owner string `toml:"owner"`
	Repo  string `toml:"repo"`
}

// GroupRef identifies a GitLab group or namespace to poll.
type GroupRef struct {
	Path string `toml:"path"`
}

// ViewConfig is a named preset of filter + sort + grouping.
type ViewConfig struct {
	Name        string       `toml:"name"`
	Resource    string       `toml:"resource"` // "pr" | "issue"
	Description string       `toml:"description"`
	Filter      FilterConfig `toml:"filter"`
	Sort        SortConfig   `toml:"sort"`
	Group       GroupConfig  `toml:"group"`
}

// FilterConfig holds all filterable fields for a view.
// Universal fields apply to all resource types; PR-specific fields are only
// valid when resource = "pr" and are rejected at load time otherwise.
type FilterConfig struct {
	// Universal filter fields
	Author       string      `toml:"author"`         // "" | "me" | username
	Repo         StringSlice `toml:"repo"`           // OR-match; "owner/name" format
	Provider     StringSlice `toml:"provider"`       // OR-match; provider name from config
	Label        StringSlice `toml:"label"`          // AND-match; PR must carry all labels
	StalenessGTE int         `toml:"staleness_days"` // >= N days since UpdatedAt; 0 = no filter
	State        string      `toml:"state"`          // "open" | "closed" | "merged" | "draft"
	TargetBranch string      `toml:"target_branch"`  // substring match

	// PR-specific filter fields
	Reviewer     string `toml:"reviewer"`      // "" | "me" | username
	Draft        *bool  `toml:"draft"`         // nil = no filter
	CIStatus     string `toml:"ci_status"`     // "passing" | "failing" | "pending" | "none"
	ReviewStatus string `toml:"review_status"` // "approved" | "changes_requested" | "review_required" | "commented" | "none"
}

// SortConfig declares sort order for a view.
type SortConfig struct {
	By        string `toml:"by"`        // "updated" | "created" | "staleness" | "title"
	Direction string `toml:"direction"` // "asc" | "desc"
}

// GroupConfig declares grouping for a view.
type GroupConfig struct {
	By string `toml:"by"` // "none" | "repo" | "provider" | "author" | "review_status"
}

// EventsConfig controls event emission behaviour.
type EventsConfig struct {
	EmitOnFirstLoad bool `toml:"emit_on_first_load"`
}

// HookConfig defines a shell command to run when a matching event fires.
type HookConfig struct {
	Event   string       `toml:"event"`
	Filter  FilterConfig `toml:"filter"`
	Command string       `toml:"command"`
}

// StringSlice unmarshals from either a single TOML string or a TOML array of
// strings. This lets config authors write `repo = "owner/name"` or
// `repo = ["owner/a", "owner/b"]` interchangeably.
type StringSlice []string

func (stringSlice *StringSlice) UnmarshalTOML(value any) error {
	switch typedValue := value.(type) {
	case string:
		*stringSlice = StringSlice{typedValue}
	case []any:
		result := make(StringSlice, len(typedValue))
		for index, element := range typedValue {
			stringElement, ok := element.(string)
			if !ok {
				return fmt.Errorf("expected string element at index %d, got %T", index, element)
			}
			result[index] = stringElement
		}
		*stringSlice = result
	default:
		return fmt.Errorf("expected string or array of strings, got %T", value)
	}
	return nil
}
