package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lstellway/prsm/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadFile_ExampleConfig verifies ../config.example.toml — the repo's
// committed reference config — loads and validates without error. It loads
// the file directly from its repo path rather than a copy, so this test is
// what keeps the example honest as the schema evolves: a config field that
// changes shape breaks this test, not just the documentation.
func TestLoadFile_ExampleConfig(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_fake")
	t.Setenv("GITLAB_TOKEN", "glpat_fake")
	t.Setenv("CODEBERG_TOKEN", "cb_fake")

	loadedConfig, err := config.LoadFile("../config.example.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(loadedConfig.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(loadedConfig.Providers))
	}
	if len(loadedConfig.Views) != 4 {
		t.Fatalf("expected 4 views, got %d", len(loadedConfig.Views))
	}
	if loadedConfig.Defaults.DefaultView != "my-reviews" {
		t.Errorf("defaults.default_view = %q, want %q", loadedConfig.Defaults.DefaultView, "my-reviews")
	}
	if loadedConfig.Providers[0].Auth.Token != "ghp_fake" {
		t.Errorf("provider[0] token after expansion = %q, want %q", loadedConfig.Providers[0].Auth.Token, "ghp_fake")
	}

	draft := loadedConfig.Views[0].Filter.Draft
	if draft == nil || *draft != false {
		t.Errorf("views[0].filter.draft = %v, want *false", draft)
	}
}

// Rule 1: unknown keys produce a warning but the load succeeds.
func TestLoadFile_UnknownKeys(t *testing.T) {
	t.Setenv("PRSM_TEST_TOK", "tok")
	content := `
[[providers]]
name    = "gh"
type    = "github"
unknown_field = "ignored"

[providers.auth]
type  = "pat"
token = "$PRSM_TEST_TOK"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err != nil {
		t.Fatalf("load should succeed despite unknown key, got: %v", err)
	}
}

// Rule 2: missing provider name fails with a descriptive error.
func TestLoadFile_MissingProviderName(t *testing.T) {
	content := `
[[providers]]
type = "github"

[providers.auth]
type  = "pat"
token = "tok"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing provider name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error %q does not mention 'name is required'", err)
	}
}

// Rule 2: missing provider type fails with a descriptive error.
func TestLoadFile_MissingProviderType(t *testing.T) {
	content := `
[[providers]]
name = "gh"

[providers.auth]
type  = "pat"
token = "tok"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing provider type, got nil")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Errorf("error %q does not mention 'type is required'", err)
	}
}

// Rule 3: missing resource on a view fails with a clear message.
func TestLoadFile_MissingViewResource(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name = "my-view"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing view resource, got nil")
	}
	if !strings.Contains(err.Error(), "resource is required") {
		t.Errorf("error %q does not mention 'resource is required'", err)
	}
}

// Rule 4: invalid sort.by fails and lists allowed values.
func TestLoadFile_InvalidSortBy(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.sort]
by = "foobar"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid sort.by, got nil")
	}
	if !strings.Contains(err.Error(), "sort.by must be one of") {
		t.Errorf("error %q does not list allowed sort.by values", err)
	}
}

// Rule 4: invalid sort.direction fails.
func TestLoadFile_InvalidSortDirection(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.sort]
by        = "updated"
direction = "sideways"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid sort.direction, got nil")
	}
	if !strings.Contains(err.Error(), "sort.direction must be one of") {
		t.Errorf("error %q does not mention sort.direction", err)
	}
}

// Rule 4: invalid provider type fails and lists allowed values.
func TestLoadFile_InvalidProviderType(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "bitbucket"
[providers.auth]
type = "pat"
token = "tok"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid provider type, got nil")
	}
	if !strings.Contains(err.Error(), "type must be one of") {
		t.Errorf("error %q does not list allowed type values", err)
	}
}

// Rule 4: invalid filter.ci_status fails.
func TestLoadFile_InvalidCIStatus(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.filter]
ci_status = "unknown"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid ci_status, got nil")
	}
	if !strings.Contains(err.Error(), "ci_status must be one of") {
		t.Errorf("error %q does not list allowed ci_status values", err)
	}
}

// Rule 4: invalid filter.review_status fails.
func TestLoadFile_InvalidReviewStatus(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.filter]
review_status = "unknown"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid review_status, got nil")
	}
	if !strings.Contains(err.Error(), "review_status must be one of") {
		t.Errorf("error %q does not list allowed review_status values", err)
	}
}

// Rule 4: invalid resource value fails with allowed values listed.
func TestLoadFile_InvalidResource(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "ticket"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid resource value, got nil")
	}
	if !strings.Contains(err.Error(), "resource must be one of") {
		t.Errorf("error %q does not list allowed resource values", err)
	}
}

// Rule 4: invalid group.by value fails with allowed values listed.
func TestLoadFile_InvalidGroupBy(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.group]
by = "priority"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid group.by value, got nil")
	}
	if !strings.Contains(err.Error(), "group.by must be one of") {
		t.Errorf("error %q does not list allowed group.by values", err)
	}
}

// Rule 4: invalid filter.state value fails with allowed values listed.
func TestLoadFile_InvalidFilterState(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "v"
resource = "pr"
[views.filter]
state = "wip"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid filter.state value, got nil")
	}
	if !strings.Contains(err.Error(), "filter.state must be one of") {
		t.Errorf("error %q does not list allowed filter.state values", err)
	}
}

// Rule 5: reviewer filter on a non-PR view fails.
func TestLoadFile_PRFilterOnIssueView(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "issues"
resource = "issue"
[views.filter]
reviewer = "me"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for PR-specific filter on issue view, got nil")
	}
	if !strings.Contains(err.Error(), "filter.reviewer is not valid for resource") {
		t.Errorf("error %q does not mention incompatible filter key", err)
	}
}

// Rule 5: review_status group key on a non-PR view fails.
func TestLoadFile_PRGroupOnIssueView(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "issues"
resource = "issue"
[views.group]
by = "review_status"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for PR-specific group key on issue view, got nil")
	}
	if !strings.Contains(err.Error(), "is not valid for resource") {
		t.Errorf("error %q does not mention resource incompatibility", err)
	}
}

// Rule 5: filter.draft on a non-PR view fails.
func TestLoadFile_DraftFilterOnIssueView(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "issues"
resource = "issue"
[views.filter]
draft = false
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for PR-specific filter.draft on issue view, got nil")
	}
	if !strings.Contains(err.Error(), "filter.draft is not valid for resource") {
		t.Errorf("error %q does not mention filter.draft incompatibility", err)
	}
}

// Rule 5: filter.ci_status on a non-PR view fails.
func TestLoadFile_CIStatusFilterOnIssueView(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "issues"
resource = "issue"
[views.filter]
ci_status = "failing"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for PR-specific filter.ci_status on issue view, got nil")
	}
	if !strings.Contains(err.Error(), "filter.ci_status is not valid for resource") {
		t.Errorf("error %q does not mention filter.ci_status incompatibility", err)
	}
}

// Rule 5: filter.review_status on a non-PR view fails.
func TestLoadFile_ReviewStatusFilterOnIssueView(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "tok"

[[views]]
name     = "issues"
resource = "issue"
[views.filter]
review_status = "approved"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for PR-specific filter.review_status on issue view, got nil")
	}
	if !strings.Contains(err.Error(), "filter.review_status is not valid for resource") {
		t.Errorf("error %q does not mention filter.review_status incompatibility", err)
	}
}

// Rule 6: duplicate provider names fail at load time.
func TestLoadFile_DuplicateProviderNames(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok1"

[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok2"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for duplicate provider name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate provider name") {
		t.Errorf("error %q does not mention 'duplicate provider name'", err)
	}
}

// Rule 7: duplicate view names fail at load time.
func TestLoadFile_DuplicateViewNames(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
token = "tok"

[[views]]
name     = "same"
resource = "pr"

[[views]]
name     = "same"
resource = "pr"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for duplicate view name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate view name") {
		t.Errorf("error %q does not mention 'duplicate view name'", err)
	}
}

// Rule 8: a $VAR token that expands to empty fails with the variable name in
// the error message.
func TestLoadFile_EmptyTokenAfterExpansion(t *testing.T) {
	// Ensure the var is unset for this test.
	t.Setenv("PRSM_TEST_UNSET_TOKEN", "")

	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "$PRSM_TEST_UNSET_TOKEN"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for empty token after env var expansion, got nil")
	}
	if !strings.Contains(err.Error(), "$PRSM_TEST_UNSET_TOKEN") {
		t.Errorf("error %q does not reference the env var name", err)
	}
	if !strings.Contains(err.Error(), "which is not set") {
		t.Errorf("error %q does not say 'which is not set'", err)
	}
}

// Rule 8: auth.password env var reference that expands to empty fails.
func TestLoadFile_EmptyPasswordAfterExpansion(t *testing.T) {
	t.Setenv("PRSM_TEST_UNSET_PWD", "")

	content := `
[[providers]]
name = "cb"
type = "gitea"
[providers.auth]
type     = "basic"
username = "myuser"
password = "$PRSM_TEST_UNSET_PWD"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for empty password after env var expansion, got nil")
	}
	if !strings.Contains(err.Error(), "$PRSM_TEST_UNSET_PWD") {
		t.Errorf("error %q does not reference the env var name", err)
	}
	if !strings.Contains(err.Error(), "which is not set") {
		t.Errorf("error %q does not say 'which is not set'", err)
	}
}

// Auth type: invalid value is rejected with allowed values listed.
func TestLoadFile_InvalidAuthType(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "github_app"
token = "tok"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for invalid auth type, got nil")
	}
	if !strings.Contains(err.Error(), "auth.type must be one of") {
		t.Errorf("error %q does not list allowed auth.type values", err)
	}
}

// Auth type: basic requires username.
func TestLoadFile_BasicAuthMissingUsername(t *testing.T) {
	content := `
[[providers]]
name = "cb"
type = "gitea"
[providers.auth]
type     = "basic"
password = "secret"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing username in basic auth, got nil")
	}
	if !strings.Contains(err.Error(), "auth.username is required") {
		t.Errorf("error %q does not mention auth.username requirement", err)
	}
}

// Auth type: basic requires password.
func TestLoadFile_BasicAuthMissingPassword(t *testing.T) {
	content := `
[[providers]]
name = "cb"
type = "gitea"
[providers.auth]
type     = "basic"
username = "myuser"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing password in basic auth, got nil")
	}
	if !strings.Contains(err.Error(), "auth.password is required") {
		t.Errorf("error %q does not mention auth.password requirement", err)
	}
}

// Auth type: pat requires token.
func TestLoadFile_PatAuthMissingToken(t *testing.T) {
	content := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type = "pat"
`
	_, err := config.LoadFile(writeTemp(t, content))
	if err == nil {
		t.Fatal("expected error for missing token in pat auth, got nil")
	}
	if !strings.Contains(err.Error(), "auth.token is required") {
		t.Errorf("error %q does not mention auth.token requirement", err)
	}
}

// Auth type: valid basic auth with env var password loads correctly.
func TestLoadFile_BasicAuthValid(t *testing.T) {
	t.Setenv("PRSM_TEST_BASIC_PWD", "secret")
	content := `
[[providers]]
name     = "cb"
type     = "gitea"
base_url = "https://codeberg.org"
[providers.auth]
type     = "basic"
username = "myuser"
password = "$PRSM_TEST_BASIC_PWD"
`
	loadedConfig, err := config.LoadFile(writeTemp(t, content))
	if err != nil {
		t.Fatalf("unexpected error for valid basic auth: %v", err)
	}
	if loadedConfig.Providers[0].Auth.Username != "myuser" {
		t.Errorf("username = %q, want %q", loadedConfig.Providers[0].Auth.Username, "myuser")
	}
	if loadedConfig.Providers[0].Auth.Password != "secret" {
		t.Errorf("password after expansion = %q, want %q", loadedConfig.Providers[0].Auth.Password, "secret")
	}
}

// StringSlice accepts both a single string and an array.
func TestFilterConfig_StringSlice(t *testing.T) {
	t.Setenv("PRSM_TEST_TOK", "tok")

	singleRepoContent := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "$PRSM_TEST_TOK"

[[views]]
name     = "v"
resource = "pr"
[views.filter]
repo = "owner/single"
`
	loadedConfig, err := config.LoadFile(writeTemp(t, singleRepoContent))
	if err != nil {
		t.Fatalf("single-string repo: %v", err)
	}
	if len(loadedConfig.Views[0].Filter.Repo) != 1 || loadedConfig.Views[0].Filter.Repo[0] != "owner/single" {
		t.Errorf("single-string repo: got %v", loadedConfig.Views[0].Filter.Repo)
	}

	multiRepoContent := `
[[providers]]
name = "gh"
type = "github"
[providers.auth]
type  = "pat"
token = "$PRSM_TEST_TOK"

[[views]]
name     = "v"
resource = "pr"
[views.filter]
repo = ["owner/a", "owner/b"]
`
	loadedConfig, err = config.LoadFile(writeTemp(t, multiRepoContent))
	if err != nil {
		t.Fatalf("array repo: %v", err)
	}
	if len(loadedConfig.Views[0].Filter.Repo) != 2 {
		t.Errorf("array repo: got %v", loadedConfig.Views[0].Filter.Repo)
	}
}
