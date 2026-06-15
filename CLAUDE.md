# prsm

Engineers on fast-moving teams miss PR review requests — buried in Slack threads, invisible across providers and organizations. prsm gives them a live, filterable view of everything waiting for their attention, so nothing important gets missed.

## Name

`prsm` — "prs" (pull requests) + evokes "prism" (seeing clearly, refracting complexity into clarity). Vowel-drop follows CLI naming convention.

## Core Philosophy

- **Resource aggregation API** — prsm's core is a provider-agnostic resource aggregation layer. Provider adapters normalize data from multiple git hosts into a unified schema. The query layer (filter, sort, group) operates on that schema. Consumers compose the model and query layers without modification.
- **TUI as first consumer, not product identity** — the terminal UI is prsm's first consumer. MCP server, HTTP API, and library distribution are first-class planned consumers, not afterthoughts. Build the resource model and query layer as if all consumers exist simultaneously.
- **Digest over manage** — prsm tells you what needs attention and gives you the context to prioritize it. Your browser handles the actual review. Write/action features (approve, comment, merge) are a future layer, not the foundation.
- **Generic schema** — normalize data from all providers into one internal model; views are provider-agnostic.
- **Multi-provider** — GitHub, GitLab, Gitea/Forgejo as first-class citizens. Provider parity is a commitment, not a roadmap item.
- **Triage-oriented** — inspired by medical triage: what needs attention, what's urgent, what's blocked.

## Architecture

Four layers with strict downward-only dependencies:

```
Provider Adapters  →  Resource Model  →  Query Layer  →  Consumers
```

- **Provider Adapters** — one per provider kind (GitHub, GitLab, Gitea/Forgejo). Fetches raw API data, normalizes it into the resource model. All provider-specific knowledge lives here.
- **Resource Model** — normalized Go types (`PullRequest`, `Issue`, …). `LoadResult[T]` for lazy-fetched fields. No presentation or transport concerns.
- **Query Layer** — resource-typed `FilterSpec`, generic `Predicate[T]`, sort and group keys. Consumer-agnostic: TUI, MCP server, HTTP API, and library all use the same pipeline.
- **Consumers** — TUI (v1), MCP server, HTTP API, library. Each adds a thin assembly and transport layer. No consumer modifies the layers below it.

See `docs/decisions/ADR-000-system-architecture.md` for the full specification.

## Tech Stack

- **Language:** Go
- **TUI framework:** Bubble Tea v2 + Lip Gloss v2 (Charmbracelet)
- **Config:** TOML via `github.com/BurntSushi/toml`
- **Provider clients:** `google/go-github` (GitHub), `gitlab-org/api/client-go` (GitLab), `go-gitea/go-sdk` (Gitea/Forgejo)

See `docs/decisions/ADR-001-tech-stack.md` for rationale.

## Providers (v1)

Implementation order:

1. **GitHub** (github.com + GitHub Enterprise Server) — establishes the adapter interface pattern
2. **GitLab** (gitlab.com + self-hosted) — follows the same adapter interface
3. **Gitea/Forgejo** — single adapter covering Gitea instances, Forgejo instances, and Codeberg

Codeberg is a Forgejo instance; it does not require a separate adapter. See `docs/decisions/ADR-002-v1-providers.md`.

## Resource Types

- **v1:** Pull requests (`PullRequest`)
- **Planned:** Issues (`Issue`) — GitHub Issues, GitLab Issues, Gitea Issues, Jira, Linear, etc.

Resource types are first-class equals. Adding a new resource type means defining its normalized model type, a resource-typed `FilterSpec`, and provider adapter methods. No other layer changes.

## Inspiration

- **k9s** — TUI for Kubernetes; the gold standard for resource-oriented terminal UIs. Uses tview/tcell internally.
- **lazygit** — 76k stars; proof that terminal-native developer tools achieve massive adoption via dotfiles/word-of-mouth. Design model for keybindings and multi-panel layout. Uses a custom gocui fork.
- **herdr** — TUI for infrastructure management; built with Ratatui.
- **Superhuman** — proof that triage-as-a-product is a viable, high-value category. "Split inboxes," speed as a first-class constraint, attention routing as the core job.
- The "PR inbox" mental model — prsm is an email client for pull requests: every item has a defined next action, sections surface priority signals before general triage.

## Design Principles (TUI)

- **Vim keybindings as default** — `hjkl` navigation, `/` to filter, `?` for contextual help.
- **Single-key operations** — multi-step actions collapse to one keystroke. This is the TUI's core value over a CLI.
- **Multi-panel layout** — PR list and PR detail in sync; navigating the list updates the context pane without leaving the tool.
- **Speed as a constraint** — every interaction should be measurably faster than the alternative (browser tab-switching, `gh pr list`, Slack scanning).

## Non-Goals (v1)

- Inline review actions (approve, comment, merge) — prsm surfaces what to review; the web UI does the review
- Replacing the web UI or becoming a full GitHub/GitLab client
- Issue resource type — architecture is designed for it; implementation is post-v1

## Decisions

Key decisions are documented in `docs/decisions/`. Read the relevant ADR before implementing anything it covers.

- [ADR-000: System Architecture](docs/decisions/ADR-000-system-architecture.md)
- [ADR-001: Tech Stack](docs/decisions/ADR-001-tech-stack.md)
- [ADR-002: v1 Provider Set](docs/decisions/ADR-002-v1-providers.md)
- [ADR-003: Liveness Model](docs/decisions/ADR-003-liveness-model.md)
- [ADR-004: PR Data Model](docs/decisions/ADR-004-pr-data-model.md)
- [ADR-005: Config Format](docs/decisions/ADR-005-config-format.md)
- [ADR-006: Filtering and Grouping](docs/decisions/ADR-006-filtering-grouping.md)
- [ADR-007: Event Engine and Hook System](docs/decisions/ADR-007-event-engine.md)

## Current State

| Package | Status | Description |
|---|---|---|
| `model/` | Done | Normalized resource types — `PullRequest`, `LoadResult[T]`, reviews, CI, diff |
| `adapter/` | Done | Provider adapter interface + stub implementations (GitHub, GitLab, Gitea, mock) |
| `config/` | Done | TOML loader, XDG path, validation (all 8 ADR-005 rules), first-run scaffold |
| `query/` | Stub | Filter, sort, group pipeline — not yet implemented |
| `client/` | Stub | — |
| `event/` | Stub | Event engine (planned v1.1 per ADR-007) |
| `internal/hook/` | Stub | Shell hook runner |
| `internal/tui/` | Stub | Bubble Tea TUI consumer |
| `cmd/prsm/` | Stub | CLI entrypoint |
