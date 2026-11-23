# Terminal UI

The TUI is prsm's first consumer of the assembly layer (`package prsm`). It adds presentation only: fetching, filtering, sorting, and grouping come from the layers below, never re-implemented here. Scope is tracked in #18 (live list) and #30 (filtering, views, detail pane). Development data comes from the fake provider (#47), not from stubbed vendor APIs.

## Design principles

- **The PR inbox** — prsm is an email client for pull requests: every item has a defined next action, and sections surface priority signals before general triage.
- **Vim keybindings as default** — `hjkl` navigation, `/` to filter, `?` for contextual help.
- **Single-key operations** — multi-step actions collapse to one keystroke. This is the TUI's core value over a CLI.
- **Multi-panel layout** — PR list and PR detail in sync; navigating the list updates the context pane without leaving the tool.
- **Speed as a constraint** — every interaction should be measurably faster than the alternative (browser tab-switching, `gh pr list`, Slack scanning).
- **Degraded state is visible** — a connection that is offline, rate limited, or unauthorized is marked on screen, never hidden. An action a connection cannot perform is shown disabled with its reason, never as a dead keystroke.

## Reference

- **k9s** — TUI for Kubernetes; the gold standard for resource-oriented terminal UIs.
- **lazygit** — the design model for keybindings and multi-panel layout.
- **lazyworktree** — Go + Bubble Tea TUI for git worktrees that shows CI/PR status per worktree. Closest reference for this stack, and a working example of rendering CI/PR state in Bubble Tea.
- **gitui** — benchmark for speed on huge repos and for keyboard-only, async-first navigation.
- **Superhuman** — triage as a product: split inboxes, speed as a first-class constraint, attention routing as the core job.
