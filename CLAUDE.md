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

Five layers with strict downward-only dependencies, plus a shared assembly layer between them and the consumers:

```
Provider Adapters  →  Resource Model  →  Query Layer  →  Event Engine  →  Assembly  →  Consumers
```

- **Provider Adapters** — one per provider kind (GitHub, GitLab, Gitea/Forgejo). Fetches raw API data, normalizes it into the resource model. All provider-specific knowledge lives here.
- **Resource Model** — normalized Go types (`PullRequest`, `Issue`, …). `LoadResult[T]` for lazy-fetched fields. No presentation or transport concerns.
- **Query Layer** — resource-typed `FilterSpec`, generic `Predicate[T]`, sort and group keys. Consumer-agnostic: TUI, MCP server, HTTP API, and library all use the same pipeline.
- **Event Engine** — diffs successive snapshots into typed events and dispatches them to subscribers and shell hooks. Planned v1.1 (ADR-007).
- **Assembly** — `package prsm` at the module root. Constructs adapters from config, resolves identities, fans out fetches, aggregates partial failures, and drives the poll cycle. Shared by every consumer; no consumer re-implements it (ADR-009).
- **Consumers** — TUI (v1), MCP server, HTTP API, library. Each adds only transport or presentation. No consumer modifies the layers below it.

See `docs/decisions/ADR-000-system-architecture.md` for the full specification and `ADR-009` for the assembly layer.

## Integration Surfaces

prsm supports integrators through two parallel, equally first-class surfaces. They must stay semantically consistent — a snapshot means the same thing in both, including degraded-provider state.

- **Go library** — `github.com/lstellway/prsm`. The integrator embeds prsm's aggregation in their own process, with their own config and credentials.
- **Wire API** — `prsm serve`, defined by `api/proto/prsm/v1`. The integrator queries a prsm instance someone else runs. `client/` is reserved for its hand-written Go SDK.

The module stays at v0.x until the TUI ships; both surfaces move to v1 together.

## Tech Stack

- **Language:** Go
- **TUI framework:** Bubble Tea v2 + Lip Gloss v2 (Charmbracelet)
- **CLI framework:** `spf13/cobra` — see `docs/decisions/research/cli-framework.md`
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
- **lazyworktree** — Go + Bubble Tea TUI for managing git worktrees; surfaces CI/PR status per worktree. Closest reference for the v1 stack (ADR-001) and a working example of rendering CI/PR state in Bubble Tea.
- **gitui** — 22k-star Rust/Ratatui git TUI; benchmark for speed on huge repos (Linux kernel in seconds) and keyboard-only, async-first navigation. Reference for the "speed as a constraint" principle and responsive async data loading.
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
- [ADR-008: Adapter Constructor Inputs](docs/decisions/ADR-008-adapter-constructor-inputs.md)
- [ADR-009: Assembly Layer and Library Surface](docs/decisions/ADR-009-assembly-layer-and-library-surface.md)

Exploratory research that predates or supports these decisions lives in `docs/decisions/research/` — notably `cli-framework.md` (the cobra decision), `project-structure.md`, and the multi-resource query notes. It is not indexed above because it records investigation rather than accepted decisions.

## Current State

**Nothing is wired end-to-end yet.** The layers below are built and tested in isolation; no production code path constructs an adapter or fetches a pull request. The assembly layer (ADR-009) is the missing piece that connects them, and `prsm tui` currently prints "not yet implemented."

| Package | Status | Description |
|---|---|---|
| `model/` | Done | Normalized resource types — `PullRequest`, `LoadResult[T]`, reviews, CI, diff |
| `adapter/` | Done | `ProviderAdapter` interface, shared `RepoRef`, error types |
| `adapter/github/` | Done | Full GitHub + GHE implementation — list, CI, reviews, diff, ETag caching, rate limiting |
| `adapter/gitlab/`, `adapter/gitea/` | Stub | `Config` structs only (pattern set by ADR-008); no constructors yet |
| `adapter/mock/` | Done | In-memory adapter for tests |
| `config/` | Done | TOML loader, XDG path, validation (all 8 ADR-005 rules), first-run scaffold |
| `query/` | Done | Filter, sort, group, fuzzy match, `Apply` pipeline — implemented and tested |
| `client/` | Reserved | Reserved for the wire API's Go SDK (ADR-009). Currently holds config→adapter mappers awaiting the move to `package prsm` |
| `internal/subcommand/` | Partial | cobra commands — `tui`, `serve`, `version`; `tui` and `serve` are not yet implemented |
| `cmd/prsm/` | Done | Thin `main` delegating to `internal/subcommand` |
| `api/proto/` | Stub | `prsm.v1` package declared; no services or RPCs defined, no codegen configured |
| `event/` | Stub | Event engine (planned v1.1 per ADR-007) |
| `internal/hook/` | Stub | Shell hook runner |
| `internal/poller/` | Stub | Empty — poll loop belongs to the assembly layer per ADR-009; remove |
| `internal/tui/` | Stub | Bubble Tea TUI consumer |
