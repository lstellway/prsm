# ADR-001: TUI Tech Stack

## Status
Proposed

## Context

prsm requires a TUI framework capable of: a live-updating two-panel layout (PR list + PR detail pane), vim-style keybindings, single-key operations, and concurrent HTTP polling of multiple git-hosting provider APIs (GitHub, GitLab, Gitea, Codeberg). Speed is a first-class constraint. The framework choice also dictates which API client libraries are available for those providers.

Three candidates were evaluated as of June 2026:

### Comparison Table

| Axis | Zig / OpenTUI | Go / Bubble Tea v2 | Rust / Ratatui |
|---|---|---|---|
| **Stars (GitHub)** | 11.8k | 43.1k | 21k |
| **Latest release** | v0.4.1 (Jun 11 2026) | v2.0.7 (Jun 1 2026) | v0.30.1 (Jun 5 2026) |
| **Total releases** | 88 | 78 | 131 |
| **Primary language for app authors** | TypeScript / JavaScript (Zig is the hidden engine) | Go | Rust |
| **Rendering model** | Shadow-buffer diff (custom, Zig-native) | Cursed Renderer (ncurses-inspired, v2) | Immediate-mode |
| **State management model** | None imposed (imperative API) | Elm Architecture (Model / Update / View) | None imposed; async template uses Tokio channels |
| **Async / live-update story** | Node.js / Bun async; Zig core is sync | Tea commands + goroutines; Mode 2026 synchronized output | Tokio mpsc channels; dedicated render tick + event tick |
| **Multi-panel support** | Yes (Flexbox subset in Zig) | Yes (compose sub-models; each gets Update+View) | Yes (Layout API, split panes) |
| **Vim keybinding support** | Manual | Manual (many examples in ecosystem) | modalkit-ratatui crate; edtui widget |
| **Ecosystem maturity** | Young; one primary production user (OpenCode/SST) | Very large: 25k+ open-source dependents; NVIDIA, Azure, AWS, GitHub, Slack | Growing: 2,100+ crates; Netflix, OpenAI, AWS, Vercel |
| **Provider API clients** | None (wrong language for GitHub/GitLab SDKs) | google/go-github (11.3k stars, v88, May 2026); official gitlab-org/api/client-go | octocrab (GitHub, Rust); no mature Gitea/Forgejo Rust client |
| **Language stability** | Zig pre-1.0 (0.15.x); breaking changes each release; 1.0 expected 2026-2027 | Go 1.x; stable, backward-compatible | Rust 1.x; stable, backward-compatible |
| **Notable production tools** | OpenCode (SST), terminal.shop | Crush (Charm AI agent), Azure Aztify, AWS eks-node-viewer, CockroachDB mc client | gitui, gitu, blippy (keyboard-first PR/issue TUI), bottom, ATAC, Yazi |
| **Inspiration tools use it?** | No (k9s=tview, lazygit=gocui, herdr=Ratatui) | No (k9s=tview, lazygit=gocui) | Yes — herdr uses Ratatui |
| **Learning curve** | High: requires JS/TS + Zig build toolchain; unusual for Go/Rust devs | Low-medium: Elm pattern is well-documented; Go is widely known | Medium: Rust ownership model; immediate-mode mental shift |

### Candidate Summaries

**Zig / OpenTUI** is primarily a TypeScript/JavaScript framework with a Zig-native rendering engine under the hood. It was built by the SST/Anomaly team to replace Bubble Tea for OpenCode after hitting performance and ergonomics limits with Ink (the React-for-terminals library). Its C ABI means it can theoretically be used from any language, but no production Go or Rust bindings exist. The Zig language itself is pre-1.0 with breaking changes between releases. For a Go or Rust author, OpenTUI offers no practical path: you'd be writing a TypeScript CLI tool, which brings a Node.js/Bun runtime dependency and eliminates the Go and Rust API client ecosystems that prsm needs. Ruled out.

**Go / Bubble Tea v2** is the dominant Go TUI framework with 43k stars and 25k+ dependents. The February 2026 v2 release added a high-performance ncurses-inspired renderer (Mode 2026 synchronized output, flicker-free), made the API fully declarative, and was pre-validated in production via Charm's own Crush AI agent. Multi-panel layouts compose naturally by nesting sub-models. Go's API client ecosystem is excellent for prsm's needs: `google/go-github` (v88, Google-maintained), `gitlab-org/api/client-go` (official GitLab SDK), and Gitea has a Go SDK (`go-gitea/go-sdk`). The Elm Architecture's explicit message-passing also makes it straightforward to fan out concurrent API polls via goroutines and feed results back as tea.Msg values without complex synchronization. The downside is that k9s (the primary TUI inspiration) uses tview/tcell rather than Bubble Tea, so there is no direct "look at k9s's source" shortcut — but Bubble Tea's ecosystem has many multi-panel examples (Crush, various dashboard tools).

**Rust / Ratatui** is the framework herdr (a direct inspiration) uses. At 21k stars and v0.30.1 it is active and healthy. Its immediate-mode rendering with Tokio async yields sub-millisecond frame times and clean separation of the event loop, state mutations, and render pass. The `modalkit-ratatui` crate provides a full Vim modal keybinding engine out of the box. `blippy` — a keyboard-first TUI for GitHub PRs and issues — was built with Ratatui and is the closest existing analog to prsm. However, the Rust API client story for multi-provider support is weaker: `octocrab` covers GitHub well, but there is no mature Rust client for Gitea/Forgejo (a stated target provider), and the GitLab Rust SDK is less established than Go's official client. Multi-provider normalization requires building more HTTP integration from scratch. Rust's ownership model also imposes a steeper learning curve than Go and makes state sharing across async tasks more verbose (Arc<Mutex<...>> or channels everywhere).

## Decision

**Go / Bubble Tea v2** is the recommended stack.

The primary reason is the combination of mature multi-provider API client libraries and a framework that models prsm's exact use case — live-updating, multi-panel, message-driven TUI — with the least accidental complexity. Go's goroutine-per-provider polling pattern maps cleanly onto the Elm message loop: each provider's goroutine sends a `tea.Msg` when new data arrives, and the single Update function reconciles state without explicit locking. The v2 release (February 2026) closed the main performance gap versus Ratatui for this workload. The ecosystem has 25k+ dependents and enterprise backing, reducing framework abandonment risk. Zig/OpenTUI is ruled out because it targets a different language runtime entirely. Rust/Ratatui is viable but carries higher per-provider HTTP integration cost and a steeper ramp-up.

## Consequences

- **Language**: prsm is a Go project. This means `go.mod`, standard Go toolchain, and the Go module ecosystem.
- **TUI**: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, and `charm.land/bubbles/v2` (import paths changed in v2). The Elm Architecture (Model/Update/View) structures all UI state and event handling.
- **Provider clients**: `google/go-github` for GitHub; `gitlab.com/gitlab-org/api/client-go` for GitLab; `code.gitea.io/sdk/gitea` for Gitea/Codeberg. Authentication follows `golang.org/x/oauth2` for all providers.
- **Async updates**: Each provider runs a goroutine that polls on a configurable interval and sends `tea.Msg` values back to the program. No shared mutable state; message-passing only.
- **Risks to monitor**:
  - Bubble Tea v2 changed import paths to vanity domains (`charm.land/...`); ensure the project pins v2 from the start to avoid a painful v1→v2 migration later.
  - The Charm organization is a small company; watch for maintenance continuity. The large dependent count (25k+) reduces — but does not eliminate — abandonment risk.
  - If Gitea/Codeberg APIs diverge significantly from GitHub's, the normalization layer will be the main engineering challenge regardless of framework choice.
