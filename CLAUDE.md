# prsm

Engineers lose time to code resources scattered across vendors, accounts, and machines — pull requests on one host, CI runs on another, branches and worktrees on disk. prsm centralizes them into one live, filterable, keyboard-driven surface, and lets you act on them from there.

v1 ships the pull-request slice; see Non-Goals.

## Name

`prsm` — "prs" (pull requests) + evokes "prism" (seeing clearly, refracting complexity into clarity). Vowel-drop follows CLI naming convention.

## Core Philosophy

- **Resource aggregation API** — prsm's core is a vendor-agnostic resource aggregation layer. Adapters normalize data from many vendors — git hosts, CI systems, the local checkout — into a unified schema. The query layer (filter, sort, group) operates on that schema. Consumers compose the model and query layers without modification.
- **TUI as first consumer, not product identity** — the terminal UI is prsm's first consumer. MCP server, HTTP API, and library distribution are first-class planned consumers, not afterthoughts. Build the resource model and query layer as if all consumers exist simultaneously.
- **Manage, don't just watch** — every resource prsm shows, it can act on: merge, comment, label, request review, rerun, check out, close. You should not need the vendor's web UI for the routine loop.
- **Honest about what it can't do** — capability is a property of a *connection*, not a vendor: the same Gitea at 1.19 and 1.24 answer differently. prsm probes each connection and says explicitly what is unavailable and why. Never a silent no-op, never a dead keystroke. Review authoring is the known permanent gap — GitHub reviews carry a body and a request-changes state, GitLab approvals carry neither — and prsm links out rather than pretending otherwise.
- **Generic schema** — normalize data from all vendors into one internal model; views are vendor-agnostic.
- **Multi-vendor, sparse by nature** — a vendor serves some resource kinds and not others. Jenkins and CircleCI have CI runs but no pull requests; the local git checkout has branches and worktrees but no host and no credential. Parity is a commitment *per resource kind* — if prsm supports pull requests on two vendors it supports them equivalently — not a claim that every vendor serves everything.
- **Vendors are built in** — adapters compile into prsm, one per vendor. No plugin runtime. Every comparable tool that reached broad vendor coverage *with writes* did it this way (Renovate: 11 code hosts; rclone: ~70 backends). Third parties extend prsm through the wire API and config-declared shell-out actions, not by shipping vendors.
- **Triage-oriented** — inspired by medical triage: what needs attention, what's urgent, what's blocked.

## Architecture

Five layers with strict downward-only dependencies, plus a shared assembly layer between them and the consumers:

```
Provider Adapters  →  Resource Model  →  Query Layer  →  Event Engine  →  Assembly  →  Consumers
```

- **Provider Adapters** — one per vendor (GitHub, GitLab, Gitea/Forgejo, Jenkins, …, plus the local git checkout). Fetches raw data, normalizes it into the resource model, and reports what the specific connection can do. All vendor-specific knowledge lives here.
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

## Code Conventions

### Naming

**Names say what the thing holds. A reader should not have to scroll up to decode a name.**

This is a deliberate departure from common Go practice, which favors very short receivers and locals. prsm optimizes for a reader meeting the code cold, not for the person who just wrote it. Long names are cheap; re-deriving what `a` or `mu` means is not.

- **Receivers** are named for their type, not initialed: `githubAdapter *GitHubAdapter`, `mockAdapter *MockAdapter`, `loadResult LoadResult[T]`. Never `a`, `m`, `r`, `self`, or `this`.
- **Locals and parameters** are whole words: `mutex` not `mu`, `instance` not `inst`, `response` not `resp`, `listOptions` not `opts`, `pullRequests` not `prs`, `testCase` not `tc`, `server` not `ts`, `rateLimitErr` not `rlErr`.
- **Struct fields** follow the same rule.
- **Loop variables** get real names: `for _, pullRequest := range pullRequests`, `for index := range items`.

**Established exceptions** — these are names in their own right, not abbreviations the reader has to expand. Do not "fix" them:

- `err` — error values
- `ctx` — `context.Context` (`context` collides with the package)
- `ok` — the comma-ok idiom
- Standard-library conventions where the signature is fixed by an interface, e.g. `t *testing.T`
- Receiver-free single-expression math where a letter *is* the domain term (rare; prefer a word)

**Watch for package-name collisions.** A descriptive name can shadow an imported package. `adapter` is the obvious trap — `github.com/lstellway/prsm/adapter` is imported by every adapter file, so a receiver or local named `adapter` makes `adapter.RepoRef` and `adapter.RateLimitError` unreachable in that scope. Same for `config`, `model`, `query`, `event`. Name for the concrete type instead (`githubAdapter`), or for the role (`provider`).

`adapter/github/github.go` is the reference implementation of this convention.

### Formatting

`gofmt` is not optional. Run `gofmt -l .` before committing; it must print nothing.

## Providers (v1)

Implementation order:

1. **GitHub** (github.com + GitHub Enterprise Server) — establishes the adapter interface pattern
2. **GitLab** (gitlab.com + self-hosted) — follows the same adapter interface
3. **Gitea/Forgejo** — single adapter covering Gitea instances, Forgejo instances, and Codeberg

Codeberg is a Forgejo instance and shares the Gitea adapter today. Forgejo forked from Gitea and their APIs are drifting; treat the shared adapter as current fact, not a permanent guarantee. See `docs/decisions/ADR-002-v1-providers.md`.

## Resource Types

- **v1:** Pull requests (`PullRequest`)
- **Planned:** Actions (`Action` — CI runs, from GitHub, Gitea, Jenkins, CircleCI, …), Branches, Worktrees, Issues

Resource types are first-class equals. Adding a new resource type means defining its normalized model type, a resource-typed `FilterSpec`, and vendor adapter methods. No other layer changes.

That is the target, not the current state. The query layer is still concrete on `PullRequest` throughout; making it generic is prerequisite work, not a free extension.

## Inspiration

- **k9s** — TUI for Kubernetes; the gold standard for resource-oriented terminal UIs. Uses tview/tcell internally.
- **lazygit** — 76k stars; proof that terminal-native developer tools achieve massive adoption via dotfiles/word-of-mouth. Design model for keybindings and multi-panel layout. Uses a custom gocui fork.
- **herdr** — Rust/Ratatui terminal multiplexer for coding-agent CLIs. Relevant for its vendor model, not its domain: per-vendor behavior lives in declarative TOML manifests that ship independently of the binary behind an engine-version handshake, while vendor *identity* stays compiled. The fallback reference if prsm's capability tables ever outgrow the release cadence.
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

- Resource kinds beyond pull requests — Actions, Branches, Worktrees and Issues are all intended; only PRs ship in v1
- Local-git resources (branches, worktrees) — the source abstraction admits an unauthenticated, hostless local source; implementation is post-v1
- Review authoring (approve, request changes) — not normalizable across vendors. A permanent non-goal, not a sequencing one
- A plugin runtime — vendors are compiled in

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
