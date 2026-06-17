package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// LoadFile decodes and validates a TOML config file. If path is empty the XDG
// default path is used. Unknown keys produce warnings on stderr but do not
// prevent a successful load (forward-compatibility). All other validation
// errors are fatal.
func LoadFile(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	var cfg Config
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	// Rule 1: unknown keys → warn, continue.
	for _, key := range meta.Undecoded() {
		fmt.Fprintf(os.Stderr, "config: warning: unknown key %q\n", key)
	}

	// Capture raw auth values before env var expansion so error messages can
	// report the original $VAR reference (rule 8).
	origTokens := make([]string, len(cfg.Providers))
	origPasswords := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		origTokens[i] = p.Auth.Token
		origPasswords[i] = p.Auth.Password
	}

	expandSecrets(&cfg)

	if err := validate(&cfg, origTokens, origPasswords); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate enforces all semantic rules defined in ADR-005 §"Validation and
// error reporting at startup". The origTokens/origPasswords slices hold the
// pre-expansion auth values indexed by provider position.
func validate(cfg *Config, origTokens, origPasswords []string) error {
	// ---- providers --------------------------------------------------------

	seen := make(map[string]bool, len(cfg.Providers))
	for i, p := range cfg.Providers {
		// Rule 2: missing required fields.
		if p.Name == "" {
			return fmt.Errorf("config: providers[%d]: name is required", i)
		}
		if p.Type == "" {
			return fmt.Errorf("config: provider %q: type is required", p.Name)
		}

		// Rule 4: invalid enum for type.
		validTypes := []string{"github", "gitlab", "gitea"}
		if !sliceContains(validTypes, p.Type) {
			return fmt.Errorf("config: provider %q: type must be one of: %s", p.Name, strings.Join(validTypes, ", "))
		}

		// Rule 6: duplicate provider names.
		if seen[p.Name] {
			return fmt.Errorf("config: duplicate provider name %q", p.Name)
		}
		seen[p.Name] = true

		// base_url must use HTTPS to prevent PAT exfiltration over plaintext.
		if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "https://") {
			return fmt.Errorf("config: provider %q: base_url must use https:// (got %q)", p.Name, p.BaseURL)
		}

		// Rule 8: empty token after env var expansion.
		if i < len(origTokens) {
			orig := origTokens[i]
			if strings.HasPrefix(orig, "$") && p.Auth.Token == "" {
				return fmt.Errorf("config: provider %q: auth.token references %s which is not set", p.Name, orig)
			}
		}
		if i < len(origPasswords) {
			orig := origPasswords[i]
			if strings.HasPrefix(orig, "$") && p.Auth.Password == "" {
				return fmt.Errorf("config: provider %q: auth.password references %s which is not set", p.Name, orig)
			}
		}

		// Rule 4: invalid auth type.
		validAuthTypes := []string{"pat", "oauth", "basic"}
		if p.Auth.Type == "" {
			return fmt.Errorf("config: provider %q: auth.type is required", p.Name)
		}
		if !sliceContains(validAuthTypes, p.Auth.Type) {
			return fmt.Errorf("config: provider %q: auth.type must be one of: %s", p.Name, strings.Join(validAuthTypes, ", "))
		}

		// Cross-field: credentials required by auth type.
		switch p.Auth.Type {
		case "pat", "oauth":
			if p.Auth.Token == "" {
				return fmt.Errorf("config: provider %q: auth.token is required for auth type %q", p.Name, p.Auth.Type)
			}
		case "basic":
			if p.Auth.Username == "" {
				return fmt.Errorf("config: provider %q: auth.username is required for auth type \"basic\"", p.Name)
			}
			if p.Auth.Password == "" {
				return fmt.Errorf("config: provider %q: auth.password is required for auth type \"basic\"", p.Name)
			}
		}
	}

	// ---- views ------------------------------------------------------------

	seenViews := make(map[string]bool, len(cfg.Views))
	for _, v := range cfg.Views {
		// Rule 2: missing required fields.
		if v.Name == "" {
			return fmt.Errorf("config: a view is missing required field \"name\"")
		}

		// Rule 3: missing resource.
		if v.Resource == "" {
			return fmt.Errorf("config: view %q: resource is required (\"pr\", \"issue\", ...)", v.Name)
		}

		// Rule 4: invalid resource value.
		validResources := []string{"pr", "issue"}
		if !sliceContains(validResources, v.Resource) {
			return fmt.Errorf("config: view %q: resource must be one of: %s", v.Name, strings.Join(validResources, ", "))
		}

		// Rule 7: duplicate view names.
		if seenViews[v.Name] {
			return fmt.Errorf("config: duplicate view name %q", v.Name)
		}
		seenViews[v.Name] = true

		// Rule 4: invalid sort.by.
		if v.Sort.By != "" {
			validSortBy := []string{"updated", "created", "staleness", "title"}
			if !sliceContains(validSortBy, v.Sort.By) {
				return fmt.Errorf("config: view %q: sort.by must be one of: %s", v.Name, strings.Join(validSortBy, ", "))
			}
		}

		// Rule 4: invalid sort.direction.
		if v.Sort.Direction != "" {
			if !sliceContains([]string{"asc", "desc"}, v.Sort.Direction) {
				return fmt.Errorf("config: view %q: sort.direction must be one of: asc, desc", v.Name)
			}
		}

		// Rule 4: invalid group.by.
		if v.Group.By != "" {
			validGroupBy := []string{"none", "repo", "provider", "author", "review_status"}
			if !sliceContains(validGroupBy, v.Group.By) {
				return fmt.Errorf("config: view %q: group.by must be one of: %s", v.Name, strings.Join(validGroupBy, ", "))
			}
		}

		// Rule 4: invalid filter.state.
		if v.Filter.State != "" {
			validStates := []string{"open", "closed", "merged", "draft"}
			if !sliceContains(validStates, v.Filter.State) {
				return fmt.Errorf("config: view %q: filter.state must be one of: %s", v.Name, strings.Join(validStates, ", "))
			}
		}

		// Rule 4: invalid filter.ci_status.
		if v.Filter.CIStatus != "" {
			validCIStatuses := []string{"passing", "failing", "pending", "none"}
			if !sliceContains(validCIStatuses, v.Filter.CIStatus) {
				return fmt.Errorf("config: view %q: filter.ci_status must be one of: %s", v.Name, strings.Join(validCIStatuses, ", "))
			}
		}

		// Rule 4: invalid filter.review_status.
		if v.Filter.ReviewStatus != "" {
			validReviewStatuses := []string{"approved", "changes_requested", "review_required", "commented", "none"}
			if !sliceContains(validReviewStatuses, v.Filter.ReviewStatus) {
				return fmt.Errorf("config: view %q: filter.review_status must be one of: %s", v.Name, strings.Join(validReviewStatuses, ", "))
			}
		}

		// Rule 5: resource-incompatible filter/group keys.
		if v.Resource != "pr" {
			if v.Filter.Reviewer != "" {
				return fmt.Errorf("config: view %q: filter.reviewer is not valid for resource %q", v.Name, v.Resource)
			}
			if v.Filter.Draft != nil {
				return fmt.Errorf("config: view %q: filter.draft is not valid for resource %q", v.Name, v.Resource)
			}
			if v.Filter.CIStatus != "" {
				return fmt.Errorf("config: view %q: filter.ci_status is not valid for resource %q", v.Name, v.Resource)
			}
			if v.Filter.ReviewStatus != "" {
				return fmt.Errorf("config: view %q: filter.review_status is not valid for resource %q", v.Name, v.Resource)
			}
			if v.Group.By == "review_status" {
				return fmt.Errorf("config: view %q: group.by %q is not valid for resource %q", v.Name, v.Group.By, v.Resource)
			}
		}
	}

	return nil
}

func sliceContains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
