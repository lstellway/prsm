# ADR-005: Configuration Format

## Status

Proposed

## Context

prsm requires a user-writable configuration file covering three concerns:

1. **Provider instances** — one or more named connections to GitHub, GitLab, or Gitea/Forgejo, each with its own credentials and (for self-hosted) base URL.
2. **Secrets handling** — tokens and passwords must be stored or referenced in a way that balances security with developer ergonomics.
3. **View definitions** — named, saveable combinations of filter + sort + grouping that users activate from within the TUI.

This ADR also records the choice of config file location and the Go library that will parse the config at startup.

---

### Format candidates

#### TOML

TOML (Tom's Obvious, Minimal Language) was designed specifically to be a human-writable config format. It has strong typing (strings, integers, booleans, datetimes, arrays, inline tables), mandatory comments support (`#`), and a straightforward mapping to Go structs. The `[[array of tables]]` syntax provides an idiomatic way to represent an ordered list of named records — exactly what a list of provider instances requires.

Go library: `github.com/BurntSushi/toml` (v1.5.0 as of mid-2026) is the de-facto standard. It is battle-tested, used across the Go ecosystem (Hugo, dozens of other CLIs), and provides `DecodeFile` with struct tags, typed errors, and an `Undecoded()` helper for detecting unknown keys during validation. `github.com/pelletier/go-toml/v2` is a faster alternative with a nearly identical API.

Used by: Hugo, Cargo (Rust package manager), Helix editor, many Go CLIs, `superfile` (a TUI file manager).

#### YAML

YAML is widely familiar to engineers via Kubernetes, Docker Compose, and GitHub Actions. It supports comments and multi-line strings, and anchors/aliases allow DRY reuse across the file. However:

- **Indentation-sensitivity** makes it fragile under manual edits (a misaligned space silently changes structure).
- **The Norway problem** — bare `no` parses as boolean `false` in YAML 1.1 (which Go's `gopkg.in/yaml.v3` implements). Tokens, hostnames, and view names could unexpectedly parse as booleans or nulls.
- YAML's permissive `interface{}` parsing requires the Go decoder to do more runtime type coercion, leading to harder-to-diagnose mismatches.
- k9s and lazygit both use YAML, but both also carry config-parse bugs that stem from YAML's loose typing. k9s maintains a parallel JSON Schema for its config to compensate.

Go library: `gopkg.in/yaml.v3`. Mature but requires defensive coding around type coercion.

#### JSON

Widely understood, strict typing, excellent library support. Not considered further: JSON does not support comments, making annotated config files impossible. User-facing config in JSON is a poor experience.

---

### Comparable tool conventions

| Tool | Format | Location | Secrets strategy |
|---|---|---|---|
| **k9s** | YAML | `~/Library/Application Support/k9s/` (macOS), `~/.config/k9s/` (Linux XDG) | No built-in; uses kubeconfig, which is a separate credential file |
| **lazygit** | YAML | `~/.config/lazygit/config.yml` (XDG) | Delegates to git credential helpers |
| **gh CLI** | YAML | `~/Library/Application Support/gh/` (macOS), `~/.config/gh/` (Linux XDG) | Tokens stored inline in `hosts.yml`, chmod 0600; separate hosts file |
| **herdr** | TOML | `~/.config/herdr/config.toml` (XDG) | Inline tokens; environment variable overrides documented |
| **Helix editor** | TOML | `~/.config/helix/config.toml` (XDG) | N/A (no network) |
| **Hugo** | TOML or YAML | Project-local | N/A |
| **Atuin** | TOML | `~/.config/atuin/config.toml` (XDG) | Inline or env var; server key stored separately |

The Go TUI ecosystem does not have a uniform convention, but the general direction for developer-targeted terminal tools is XDG compliance on Linux (with macOS `~/Library/Application Support/` or `~/.config/` fallback) and a human-readable text format. TOML is increasingly preferred over YAML for config — its strong typing eliminates the class of silent parse errors YAML is prone to, and engineers are familiar with it from Cargo and Helix.

---

### Secrets handling options

#### Option A: Inline plaintext

Store tokens directly in the config file. File permissions (0600, user-only read) provide the primary protection.

Pros: simplest UX; works everywhere; no external dependencies.
Cons: tokens visible in the file, in backups, and potentially in dotfile repos if the user accidentally commits the file.

Used by: `gh` CLI (`hosts.yml`), Atuin, herdr, most terminal tools.

#### Option B: Environment variable references

The config file contains a reference like `token = "$GITHUB_TOKEN"` that prsm expands at startup using `os.ExpandEnv` or `os.Getenv`.

Pros: secrets stay out of the config file; compatible with secrets managers (1Password CLI, Vault, direnv, `.env` files); CI/CD-friendly; dotfile-safe.
Cons: slightly more setup; variable must be present at launch (startup error if unset); less discoverable for new users.

Used by: Terraform (env var overrides), Docker Compose (`.env` interpolation), many Go CLIs as a complementary mechanism.

#### Option C: OS keychain integration

Tokens stored in macOS Keychain / Linux Secret Service / Windows Credential Manager via `github.com/zalando/go-keyring`. The config file references a keychain entry by name.

Pros: highest security; tokens never on disk in plaintext.
Cons: adds a native dependency with platform-specific behavior; Linux Secret Service requires a running D-Bus daemon (not available in headless/SSH environments); adds meaningful setup complexity; `go-keyring` is an additional transitive dependency; migration from inline tokens requires a `prsm auth login` flow.

Used by: `gh` CLI (offers keychain storage as an option post-`gh auth login`), Docker credential helpers.

#### Option D: Separate credentials file

Credentials in `~/.config/prsm/credentials` (chmod 0600), main config in `~/.config/prsm/config.toml`. Similar to how `gh` separates `config.yml` from `hosts.yml`.

Pros: allows committing `config.toml` (view definitions, provider URLs, display prefs) without exposing credentials; clear separation of concerns.
Cons: two files to manage; discoverability lower; tooling must handle "missing credentials file" gracefully.

**Decision on secrets:** Support both inline tokens (Option A) and environment variable references (Option B) in v1. Inline tokens are acceptable for local developer tools behind 0600 permissions — this is the established convention for `gh`, herdr, and Atuin. Env var references give security-conscious users and CI environments a path to keep tokens out of the file entirely. OS keychain integration (Option C) is deferred to a future `prsm auth login` command that would write to the keychain and update the config to reference a keychain entry. The two-file approach (Option D) is not adopted for v1: the added complexity is not worth the benefit when env var references already solve the "safe to commit config" case.

---

### Config file location

XDG Base Directory Specification:
- Config home: `$XDG_CONFIG_HOME` (defaults to `~/.config` on Linux/macOS when not set)
- prsm config path: `$XDG_CONFIG_HOME/prsm/config.toml`
- On macOS, `$XDG_CONFIG_HOME` is typically unset, so the default is `~/.config/prsm/config.toml`

This follows the same convention as lazygit, herdr, Helix, and Atuin. k9s on macOS uses `~/Library/Application Support/k9s/` (macOS-native), but this creates a split experience for users who sync dotfiles across Linux and macOS. XDG with `~/.config` is more consistent for a developer CLI tool targeting engineers who manage dotfiles.

Alternative considered: `~/.prsm/config.toml`. Rejected: pollutes the home directory with a tool-specific dotfile directory, which goes against modern conventions and is harder to manage in dotfiles repos.

---

### View definition schema

A **view** is a named preset of filter + sort + grouping that a user can activate from the TUI. Views work across all configured providers simultaneously.

Each view requires a `resource` field declaring which resource type it targets. This field is **required** — there is no default. Omitting it is a config load-time error. All resource types are equals; defaulting to `"pr"` would encode an assumption that pull requests are the canonical resource type.

```toml
[[views]]
name     = "my-reviews"
resource = "pr"       # required; "pr" | "issue" | future resource types
```

Filter keys, sort keys, and grouping keys are resource-scoped: keys valid for `"pr"` may not be valid for `"issue"` and vice versa. prsm validates key compatibility against the declared `resource` type at config load time and fails with a clear error if a mismatch is detected.

**Universal fields (valid for all resource types):**

| Field | Type | Notes |
|---|---|---|
| `author` | string or `"me"` | Filter by author. `"me"` resolves to the authenticated user per provider. |
| `repo` | string or list | Limit to specific repo(s), e.g., `"org/repo"`. OR-match across list. |
| `provider` | string or list | Limit to named provider instance(s). OR-match across list. |
| `label` | string or list | Must include all listed labels (AND-match). |
| `staleness_days` | int | Older than N days since last update. |
| `state` | string | `"open"`, `"closed"`, `"merged"`. Default: `"open"`. |

**PR-specific fields (`resource = "pr"`):**

| Field | Type | Notes |
|---|---|---|
| `reviewer` | string or `"me"` | Filter to PRs where the given user is a requested reviewer. |
| `draft` | bool | `true` = only drafts; `false` = only non-drafts; omit = both. |
| `ci_status` | string | `"passing"`, `"failing"`, `"pending"`, `"none"`. |
| `review_status` | string | `"approved"`, `"changes_requested"`, `"commented"`, `"none"`. |

**Sort orders (universal):** `updated` (default), `created`, `staleness`, `title`

**Groupings:**

- `none` — flat list (default)
- `repo` — group by repository *(universal)*
- `provider` — group by provider instance *(universal)*
- `author` — group by PR/issue author *(universal)*
- `review_status` — group by review cycle stage *(PR only)*

**View inheritance:** Views do not inherit from one another in v1. A view is a complete, self-contained preset. A `[defaults]` table in the config sets the starting state before any view is applied.

---

## Decision

### Format: TOML

TOML is chosen over YAML for prsm's config format. The key reasons:

1. TOML's strong typing eliminates silent parse errors that YAML's type coercion produces (the Norway problem is a real risk for tokens and hostnames).
2. TOML's `[[array of tables]]` syntax maps directly and readably onto the "list of provider instances" and "list of views" structure prsm needs.
3. TOML is increasingly the format of choice for developer-targeted tools in the Go and Rust ecosystems (Cargo, Helix, Hugo, herdr).
4. `github.com/BurntSushi/toml` is mature, widely used across the Go ecosystem, and provides useful validation primitives (`Undecoded()` for detecting unknown keys at startup).

Go library: `github.com/BurntSushi/toml` (v1.x). No need for Viper — prsm's config is a single file with a known schema; Viper's multi-source merging, remote config, and watch features add complexity that is not needed here.

### Secrets: inline tokens + env var references

Tokens may be written directly into the config file (file permissions 0600, created by prsm on first run). Any string value in the config may use `$VAR_NAME` syntax, which prsm expands via `os.ExpandEnv` at load time. This applies specifically to `token` fields and `password` fields. Values that do not start with `$` are used as-is.

OS keychain integration is deferred to a future `prsm auth` subcommand.

### Location: `$XDG_CONFIG_HOME/prsm/config.toml`

Default: `~/.config/prsm/config.toml`. prsm creates this directory on first run if absent. The path follows the XDG Base Directory Specification and matches the convention used by lazygit, herdr, and Helix.

---

### Complete annotated example config

```toml
# prsm configuration
# Location: ~/.config/prsm/config.toml
# Permissions: 0600 (set automatically by prsm on first write)
#
# Tokens may be written inline OR referenced as environment variables:
#   token = "ghp_actual_token_here"    # inline
#   token = "$GITHUB_TOKEN"            # env var reference (expanded at startup)

# ---------------------------------------------------------------------------
# Global defaults
# Applied before any named view is activated. Individual views override these.
# ---------------------------------------------------------------------------
[defaults]
# refresh_interval_seconds controls how often prsm polls providers for new data.
# Lower values increase API call frequency; see ADR-003 for rate limit analysis.
refresh_interval_seconds = 60

# default_view is the view prsm opens on launch. Must match a [[views]] name.
default_view = "my-reviews"

# ---------------------------------------------------------------------------
# Provider instances
# Each [[providers]] entry is one connection to a git hosting service.
# Multiple entries of the same type are allowed (e.g., github.com + GHE).
# ---------------------------------------------------------------------------

[[providers]]
# name is referenced in view definitions and displayed in the TUI header.
name        = "github-personal"
type        = "github"
# base_url defaults to "https://api.github.com" when omitted.
# Set this for GitHub Enterprise Server instances:
#   base_url = "https://github.example.com/api/v3"

# auth.type choices for GitHub: "pat" | "oauth" | "github_app"
[providers.auth]
type  = "pat"
# Token inline — keep this file chmod 0600 and out of version control.
token = "$GITHUB_TOKEN"

# repos limits polling to these repositories. If omitted, prsm fetches
# all open PRs where you are author or requested reviewer across your orgs.
[[providers.repos]]
owner = "acme-corp"
repo  = "api-service"

[[providers.repos]]
owner = "acme-corp"
repo  = "frontend"

[[providers.repos]]
owner = "loganstellway"
repo  = "dotfiles"

# ---------------------------------------------------------------------------

[[providers]]
name     = "gitlab-work"
type     = "gitlab"
# Self-hosted GitLab instance — base_url is required here.
base_url = "https://gitlab.internal.example.com"

[providers.auth]
type  = "pat"
token = "$GITLAB_WORK_TOKEN"

# groups polls all projects within these GitLab groups/namespaces.
[[providers.groups]]
path = "platform-team"

[[providers.groups]]
path = "backend"

# ---------------------------------------------------------------------------

[[providers]]
# Codeberg is a Forgejo instance — use type "gitea" to cover both.
name     = "codeberg"
type     = "gitea"
base_url = "https://codeberg.org"

[providers.auth]
type  = "pat"
token = "$CODEBERG_TOKEN"

[[providers.repos]]
owner = "loganstellway"
repo  = "public-project"

# ---------------------------------------------------------------------------
# View definitions
# Named presets of filter + sort + grouping. Activate from the TUI with the
# view picker (v) or by setting defaults.default_view above.
# ---------------------------------------------------------------------------

[[views]]
name        = "my-reviews"
resource    = "pr"   # required — "pr" | "issue" | future resource types
description = "PRs where I am a requested reviewer, non-draft only"

[views.filter]
reviewer = "me"   # "me" resolves to the authenticated user per provider
draft    = false

[views.sort]
by        = "updated"  # "updated" | "created" | "staleness" | "title"
direction = "desc"     # "asc" | "desc"

[views.group]
by = "provider"  # "none" | "repo" | "provider" | "review_status" | "author"

# ---------------------------------------------------------------------------

[[views]]
name        = "my-open-prs"
resource    = "pr"
description = "All open PRs I authored, newest first"

[views.filter]
author = "me"
status = "open"  # "open" | "closed" | "merged" — defaults to "open"

[views.sort]
by        = "created"
direction = "desc"

[views.group]
by = "repo"

# ---------------------------------------------------------------------------

[[views]]
name        = "stale-reviews"
resource    = "pr"
description = "PRs awaiting my review, untouched for 3+ days"

[views.filter]
reviewer       = "me"
draft          = false
staleness_days = 3

[views.sort]
by        = "staleness"  # least-recently-updated first
direction = "desc"

[views.group]
by = "repo"

# ---------------------------------------------------------------------------

[[views]]
name        = "ci-failures"
resource    = "pr"
description = "Any open PR in watched repos with a failing pipeline"

[views.filter]
ci_status = "failing"

[views.sort]
by        = "updated"
direction = "desc"

[views.group]
by = "repo"
```

---

## Consequences

### Go library

Add `github.com/BurntSushi/toml` as the sole config dependency. No Viper, no go-yaml. The config loader reads the file with `toml.DecodeFile`, unmarshals into a typed `Config` struct, then calls `meta.Undecoded()` to detect unknown keys and surface them as startup warnings (not fatal errors, to allow forward-compatibility with config written for a newer prsm version).

### Security implications

- prsm creates `~/.config/prsm/` with `os.MkdirAll` using permissions 0700 on first run.
- prsm creates `config.toml` with permissions 0600 when generating a starter config.
- If an existing config file has looser permissions (e.g., 0644), prsm logs a warning at startup: "config file is world-readable; consider running: chmod 0600 ~/.config/prsm/config.toml".
- Tokens written inline are visible in the file and in any backup or sync tool that mirrors `~/.config`. Users with strict security requirements should use the `$VAR_NAME` form.
- `os.ExpandEnv` is applied only to values in `auth.token` and `auth.password` fields, not to the entire config, to avoid accidental expansion of user-supplied strings (repo names, view descriptions, etc.) that happen to contain `$`.

### Env var expansion

At config load time, after TOML decode, the config loader iterates over all provider auth blocks and calls `os.ExpandEnv` on the `Token` and `Password` fields only. If the expanded value is an empty string (variable unset), prsm fails startup for that provider with a clear error: `provider "github-personal": auth.token references $GITHUB_TOKEN which is not set`.

### Migration path to OS keychain

A future `prsm auth login <provider>` command will:
1. Walk the user through provider OAuth or PAT entry.
2. Store the token in the OS keychain via `github.com/zalando/go-keyring`.
3. Write (or update) the config entry with `token = "keychain:<service>/<account>"`.
4. The config loader will detect the `keychain:` prefix and retrieve the value from the keychain instead of using it as a literal string.

This migration requires no breaking changes to the config schema — it is additive. Users who never run `prsm auth login` continue using inline or env-var tokens without disruption.

### Validation and error reporting at startup

prsm validates the decoded config before launching the TUI:

1. **Unknown keys** (`meta.Undecoded()`): warn but continue. Allows configs written for future versions to load on older prsm.
2. **Missing required fields**: fail with a descriptive error pointing to the TOML key and line number (BurntSushi/toml errors include position information).
3. **Missing `resource` field on a view**: fail at startup with: `view "my-reviews": resource is required ("pr", "issue", …)`.
4. **Invalid enum values** (e.g., `sort.by = "foobar"`): fail at startup with: `view "my-reviews": sort.by must be one of: updated, created, staleness, title`.
5. **Resource-incompatible keys** (e.g., `review_status` grouping on an Issue view): fail at startup with: `view "my-issues": group.by "review_status" is not valid for resource "issue"`.
6. **Duplicate provider names**: fail at startup — provider names are used as keys in the TUI and must be unique.
7. **Duplicate view names**: fail at startup for the same reason.
8. **Empty token after env var expansion**: fail at startup per provider, with the env var name in the message.

All startup errors are printed to stderr before the TUI initializes, so they appear as plain text rather than inside a potentially-broken TUI frame.
