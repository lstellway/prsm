# prsm

Engineers lose time to code resources scattered across vendors, accounts, and machines — pull requests on one host, CI runs on another, branches and worktrees on disk. prsm centralizes them into one live, filterable, keyboard-driven surface, and lets you act on them from there.

v1 ships the pull-request slice; see Non-Goals.

## Where context lives

- **What a package does, and what in it is not built yet** — its Go package doc comment.
- **Roadmap, design decisions, open questions** — GitHub Issues in `lstellway/prsm` (`gh issue list`, `gh issue view N --comments`). Decisions are recorded there, not in a docs directory.
- **Area-specific guidance** — `adapter/AGENTS.md` for vendor adapters, `internal/tui/AGENTS.md` for the terminal UI. Each `CLAUDE.md` is a symlink to the `AGENTS.md` beside it; edit the `AGENTS.md`.

## Core Philosophy

- **Resource aggregation API** — prsm's core is a vendor-agnostic resource aggregation layer. Adapters normalize data from many vendors — git hosts, CI systems, the local checkout — into a unified schema. The query layer (filter, sort, group) operates on that schema. Consumers compose the model and query layers without modification.
- **TUI as first consumer, not product identity** — the terminal UI is prsm's first consumer. MCP server, HTTP API, and library distribution are first-class planned consumers, not afterthoughts. Build the resource model and query layer as if all consumers exist simultaneously.
- **Manage, don't just watch** — every resource prsm shows, it can act on: merge, comment, label, request review, rerun, check out, close. You should not need the vendor's web UI for the routine loop.
- **Honest about what it can't do** — never a silent no-op, never a dead keystroke; prsm says what is unavailable and why. Review authoring is the known permanent gap — GitHub reviews carry a body and a request-changes state, GitLab approvals carry neither — and prsm links out rather than pretending otherwise.
- **Multi-vendor, sparse by nature** — one normalized model for all vendors; views are vendor-agnostic. Parity is a commitment *per resource kind*: if prsm supports pull requests on two vendors it supports them equivalently. It is not a claim that every vendor serves every kind.
- **Vendors are built in** — adapters compile into prsm, one per vendor. No plugin runtime. Third parties extend prsm through the wire API and config-declared shell-out actions, not by shipping vendors.
- **Triage-oriented** — inspired by medical triage: what needs attention, what's urgent, what's blocked.

## Vendors, connections, and resource kinds

One question drives the design: *can this source answer for this resource kind?* It has two answers, knowable at different times, needing different machinery. Do not collapse them.

**Axis 1 — does this source serve this kind at all?** Structural, settled at compile time. One Go interface per resource kind — `adapter.PullRequestSource` today, one more per kind added later. Jenkins serves CI runs and no pull requests, so it does not implement `PullRequestSource`, and the assembly layer never asks it. There is no stub returning "unsupported": a permanent structural fact must not be reported as a runtime failure. Assembly discovers what a connection serves by type assertion at construction.

**Axis 2 — for a kind it does serve, what can *this connection* do?** Runtime, knowable only by probing. The same Gitea at 1.19 and 1.24 answer differently; a read-scoped token and a write-scoped token answer differently. This is a capability set carried by the connection, plus a typed error carrying a reason.

Three kinds of "no" follow, and they must stay distinguishable end to end:

| | Mechanism | What the user sees |
|---|---|---|
| Not served | no interface — compile time | prsm never offers it |
| Served, this connection cannot | capability + typed reason | offered but disabled, with the reason |
| Served, capable, failed this time | ordinary error | offered, attempted, reported broken |

**Vocabulary.** A **vendor** is the product (GitHub). A **connection** is one configured endpoint plus a credential. A **source** is anything that serves resources — a connection, or the local git checkout, which has no host, no credential, and no account. A **resource kind** is `PullRequest`, `Action`, `Branch`, `Worktree`, `Issue`. The code still says "provider" in many places; the rename to "vendor" is #19.

Because a source may have neither host nor credential, `adapter.Connection` requires only that a source name itself. Everything beyond that — identity included — is an interface a source may decline to implement, and declining is never a failure.

Axis 1 is implemented in `adapter/adapter.go`. Axis 2 is not built yet; see #20.

## Architecture

Layers with strict downward-only dependencies, plus a shared assembly layer between them and the consumers:

```
Provider Adapters  →  Resource Model  →  Query Layer  →  Event Engine  →  Assembly  →  Consumers
```

- **Provider Adapters** (`adapter/`) — one per vendor, plus the local git checkout later. Fetches raw data, normalizes it into the resource model, and reports what the specific connection can do. All vendor-specific knowledge lives here.
- **Resource Model** (`model/`) — normalized Go types. `LoadResult[T]` for lazy-fetched fields. No presentation or transport concerns.
- **Query Layer** (`query/`) — resource-typed `FilterSpec`, generic `Predicate[T]`, sort and group keys. Consumer-agnostic: every consumer uses the same pipeline.
- **Event Engine** (`event/`) — diffs successive snapshots into typed events and dispatches them to subscribers and shell hooks. Planned v1.1.
- **Assembly** (`package prsm` at the module root) — constructs adapters from config, resolves identities, fans out fetches, aggregates partial failures, and drives the poll cycle. Shared by every consumer; no consumer re-implements it.
- **Consumers** — TUI (v1), MCP server, HTTP API, library. Each adds only transport or presentation. No consumer modifies the layers below it.

Adding a resource kind is meant to mean only a normalized model type, a resource-typed `FilterSpec`, and vendor adapter methods. That is the target, not the current state: the query layer is still concrete on `PullRequest`, and making it generic is prerequisite work.

## Integration Surfaces

Two parallel, equally first-class surfaces. They must stay semantically consistent — a snapshot means the same thing in both, including degraded-connection state.

- **Go library** — `github.com/lstellway/prsm`. The integrator embeds prsm's aggregation in their own process, with their own config and credentials.
- **Wire API** — `prsm serve`, defined by `api/proto/prsm/v1`. The integrator queries a prsm instance someone else runs. `client/` is reserved for its hand-written Go SDK.

The module stays at v0.x until the TUI ships; both surfaces move to v1 together.

## Tech Stack

- **Language:** Go
- **TUI:** Bubble Tea v2 + Lip Gloss v2 (Charmbracelet)
- **CLI:** `spf13/cobra`
- **Config:** TOML via `github.com/BurntSushi/toml`
- **Vendor clients:** `google/go-github` (GitHub), `gitlab-org/api/client-go` (GitLab), `go-gitea/go-sdk` (Gitea/Forgejo)

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

**Watch for package-name collisions.** A descriptive name can shadow an imported package. `adapter` is the obvious trap — `github.com/lstellway/prsm/adapter` is imported by every adapter file, so a receiver or local named `adapter` makes `adapter.PullRequestSource` and `adapter.RateLimitError` unreachable in that scope. Same for `config`, `model`, `query`, `event`. Name for the concrete type instead (`githubAdapter`), or for the role (`provider`).

`adapter/github/github.go` is the reference implementation of this convention.

### Formatting

`gofmt` is not optional. Run `gofmt -l .` before committing; it must print nothing.

### Build

Use `make build`, `make test`, `make lint` — they match what CI runs. A raw `go build ./...` (or `go install`) fails with `error obtaining VCS status: exit status 128` in a worktree checkout, because worktrees live as subdirectories of a bare repo here: Go's VCS stamping only recognizes a `.git` directory as a repo root, not a linked worktree's `.git` file, so it walks up to the enclosing bare directory and runs `git status` there, where it has no work tree. `make build` already passes `-buildvcs=false`; do the same for any raw `go build`/`go install` invocation. `go vet` and `go test` are unaffected.

## Non-Goals (v1)

- Resource kinds beyond pull requests — Actions, Branches, Worktrees and Issues are all intended; only PRs ship in v1
- Local-git resources (branches, worktrees) — the source abstraction admits an unauthenticated, hostless local source; implementation is post-v1
- Review authoring (approve, request changes) — not normalizable across vendors. A permanent non-goal, not a sequencing one
- A plugin runtime — vendors are compiled in
