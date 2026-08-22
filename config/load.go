package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lstellway/prsm/model"
)

// LoadFile decodes and validates a TOML config file. If path is empty the XDG
// default path is used. Unknown keys produce warnings on stderr but do not
// prevent a successful load (forward-compatibility). All other validation
// errors are fatal.
func LoadFile(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	var loadedConfig Config
	metadata, err := toml.DecodeFile(path, &loadedConfig)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	// Rule 1: unknown keys → warn, continue.
	for _, key := range metadata.Undecoded() {
		fmt.Fprintf(os.Stderr, "config: warning: unknown key %q\n", key)
	}

	// Capture raw auth values before env var expansion so error messages can
	// report the original $VAR reference (rule 8).
	originalTokens := make([]string, len(loadedConfig.Providers))
	originalPasswords := make([]string, len(loadedConfig.Providers))
	for index, provider := range loadedConfig.Providers {
		originalTokens[index] = provider.Auth.Token
		originalPasswords[index] = provider.Auth.Password
	}

	expandSecrets(&loadedConfig)

	if err := validate(&loadedConfig, originalTokens, originalPasswords); err != nil {
		return nil, err
	}

	return &loadedConfig, nil
}

// validate enforces all semantic validation rules. The originalTokens/originalPasswords
// slices hold the pre-expansion auth values indexed by provider position.
func validate(loadedConfig *Config, originalTokens, originalPasswords []string) error {
	// ---- providers --------------------------------------------------------

	seenProviderNames := make(map[string]bool, len(loadedConfig.Providers))
	for index, provider := range loadedConfig.Providers {
		// Rule 2: missing required fields.
		if provider.Name == "" {
			return fmt.Errorf("config: providers[%d]: name is required", index)
		}
		if !provider.Type.IsKnown() {
			return fmt.Errorf("config: provider %q: type is required", provider.Name)
		}

		// Rule 4: invalid enum for type.
		validTypes := model.KnownProviderKinds()
		if !sliceContains(validTypes, provider.Type) {
			return fmt.Errorf("config: provider %q: type must be one of: %s", provider.Name, joinProviderKinds(validTypes))
		}

		// Rule 6: duplicate provider names.
		if seenProviderNames[provider.Name] {
			return fmt.Errorf("config: duplicate provider name %q", provider.Name)
		}
		seenProviderNames[provider.Name] = true

		// base_url must use HTTPS to prevent PAT exfiltration over plaintext.
		if provider.BaseURL != "" && !strings.HasPrefix(provider.BaseURL, "https://") {
			return fmt.Errorf("config: provider %q: base_url must use https:// (got %q)", provider.Name, provider.BaseURL)
		}

		// Rule 8: empty token after env var expansion.
		if index < len(originalTokens) {
			originalToken := originalTokens[index]
			if strings.HasPrefix(originalToken, "$") && provider.Auth.Token == "" {
				return fmt.Errorf("config: provider %q: auth.token references %s which is not set", provider.Name, originalToken)
			}
		}
		if index < len(originalPasswords) {
			originalPassword := originalPasswords[index]
			if strings.HasPrefix(originalPassword, "$") && provider.Auth.Password == "" {
				return fmt.Errorf("config: provider %q: auth.password references %s which is not set", provider.Name, originalPassword)
			}
		}

		// Rule 4: invalid auth type.
		validAuthTypes := []string{"pat", "oauth", "basic"}
		if provider.Auth.Type == "" {
			return fmt.Errorf("config: provider %q: auth.type is required", provider.Name)
		}
		if !sliceContains(validAuthTypes, provider.Auth.Type) {
			return fmt.Errorf("config: provider %q: auth.type must be one of: %s", provider.Name, strings.Join(validAuthTypes, ", "))
		}

		// Cross-field: credentials required by auth type.
		switch provider.Auth.Type {
		case "pat", "oauth":
			if provider.Auth.Token == "" {
				return fmt.Errorf("config: provider %q: auth.token is required for auth type %q", provider.Name, provider.Auth.Type)
			}
		case "basic":
			if provider.Auth.Username == "" {
				return fmt.Errorf("config: provider %q: auth.username is required for auth type \"basic\"", provider.Name)
			}
			if provider.Auth.Password == "" {
				return fmt.Errorf("config: provider %q: auth.password is required for auth type \"basic\"", provider.Name)
			}
		}
	}

	// ---- views ------------------------------------------------------------

	seenViewNames := make(map[string]bool, len(loadedConfig.Views))
	for _, view := range loadedConfig.Views {
		// Rule 2: missing required fields.
		if view.Name == "" {
			return fmt.Errorf("config: a view is missing required field \"name\"")
		}

		// Rule 3: missing resource.
		if view.Resource == "" {
			return fmt.Errorf("config: view %q: resource is required (\"pr\", \"issue\", ...)", view.Name)
		}

		// Rule 4: invalid resource value.
		validResources := []string{"pr", "issue"}
		if !sliceContains(validResources, view.Resource) {
			return fmt.Errorf("config: view %q: resource must be one of: %s", view.Name, strings.Join(validResources, ", "))
		}

		// Rule 7: duplicate view names.
		if seenViewNames[view.Name] {
			return fmt.Errorf("config: duplicate view name %q", view.Name)
		}
		seenViewNames[view.Name] = true

		// Rule 4: invalid sort.by.
		if view.Sort.By != "" {
			validSortBy := []string{"updated", "created", "staleness", "title"}
			if !sliceContains(validSortBy, view.Sort.By) {
				return fmt.Errorf("config: view %q: sort.by must be one of: %s", view.Name, strings.Join(validSortBy, ", "))
			}
		}

		// Rule 4: invalid sort.direction.
		if view.Sort.Direction != "" {
			if !sliceContains([]string{"asc", "desc"}, view.Sort.Direction) {
				return fmt.Errorf("config: view %q: sort.direction must be one of: asc, desc", view.Name)
			}
		}

		// Rule 4: invalid group.by.
		if view.Group.By != "" {
			validGroupBy := []string{"none", "repo", "provider", "author", "review_status"}
			if !sliceContains(validGroupBy, view.Group.By) {
				return fmt.Errorf("config: view %q: group.by must be one of: %s", view.Name, strings.Join(validGroupBy, ", "))
			}
		}

		// Rule 4: invalid filter.state.
		if view.Filter.State != "" {
			validStates := []string{"open", "closed", "merged", "draft"}
			if !sliceContains(validStates, view.Filter.State) {
				return fmt.Errorf("config: view %q: filter.state must be one of: %s", view.Name, strings.Join(validStates, ", "))
			}
		}

		// Rule 4: invalid filter.ci_status.
		if view.Filter.CIStatus != "" {
			validCIStatuses := []string{"passing", "failing", "pending", "none"}
			if !sliceContains(validCIStatuses, view.Filter.CIStatus) {
				return fmt.Errorf("config: view %q: filter.ci_status must be one of: %s", view.Name, strings.Join(validCIStatuses, ", "))
			}
		}

		// Rule 4: invalid filter.review_status.
		if view.Filter.ReviewStatus != "" {
			validReviewStatuses := []string{"approved", "changes_requested", "review_required", "commented", "none"}
			if !sliceContains(validReviewStatuses, view.Filter.ReviewStatus) {
				return fmt.Errorf("config: view %q: filter.review_status must be one of: %s", view.Name, strings.Join(validReviewStatuses, ", "))
			}
		}

		// Rule 5: resource-incompatible filter/group keys.
		if view.Resource != "pr" {
			if view.Filter.Reviewer != "" {
				return fmt.Errorf("config: view %q: filter.reviewer is not valid for resource %q", view.Name, view.Resource)
			}
			if view.Filter.Draft != nil {
				return fmt.Errorf("config: view %q: filter.draft is not valid for resource %q", view.Name, view.Resource)
			}
			if view.Filter.CIStatus != "" {
				return fmt.Errorf("config: view %q: filter.ci_status is not valid for resource %q", view.Name, view.Resource)
			}
			if view.Filter.ReviewStatus != "" {
				return fmt.Errorf("config: view %q: filter.review_status is not valid for resource %q", view.Name, view.Resource)
			}
			if view.Group.By == "review_status" {
				return fmt.Errorf("config: view %q: group.by %q is not valid for resource %q", view.Name, view.Group.By, view.Resource)
			}
		}
	}

	return nil
}

func sliceContains[T comparable](candidates []T, target T) bool {
	for _, candidate := range candidates {
		if candidate == target {
			return true
		}
	}
	return false
}

// joinProviderKinds renders a validation error's list of acceptable provider
// types, e.g. "github, gitlab, gitea".
func joinProviderKinds(kinds []model.ProviderKind) string {
	names := make([]string, len(kinds))
	for index, kind := range kinds {
		names[index] = string(kind)
	}
	return strings.Join(names, ", ")
}
