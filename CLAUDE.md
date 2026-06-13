# prsm

Engineers on fast-moving teams miss PR review requests — buried in Slack threads, invisible across providers and organizations. prsm gives them a live, filterable view of everything waiting for their attention, so nothing important gets missed.

## Name

`prsm` — "prs" (pull requests) + evokes "prism" (seeing clearly, refracting complexity into clarity). Vowel-drop follows CLI naming convention.

## Core Philosophy

- **Digest over manage** — prsm tells you what needs attention and gives you the context to prioritize it. Your browser handles the actual review. Write/action features (approve, comment, merge) are a future layer, not the foundation.
- **Generic schema** — normalize data from all providers into one internal model; views are provider-agnostic
- **Multi-provider** — GitHub, GitLab, Gitea, Codeberg (and others) as first-class citizens
- **Rich views** — filters, sorts, groupings over the normalized data
- **Triage-oriented** — inspired by medical triage: what needs attention, what's urgent, what's blocked

## Planned Providers

- GitHub
- GitLab
- Gitea
- Codeberg

## Potential Extension: Issue Trackers

Same generic-schema approach applied to issue trackers: JIRA, Linear, GitHub Issues, Gitea Issues, etc. Not committed yet — keep the architecture open to it.

## Inspiration

- **k9s** — TUI for Kubernetes; the gold standard for this kind of tool
- **herdr** — TUI for infrastructure management
- **lazygit** — 76k stars; proof that terminal-native developer tools can achieve massive adoption via dotfiles/word-of-mouth. Design model for keybindings and multi-panel layout.
- **Superhuman** — proof that triage-as-a-product is a viable, high-value category. "Split inboxes," speed as a first-class constraint, attention routing as the core job.
- The "PR inbox" mental model — prsm is an email client for pull requests: every item has a defined next action, sections surface priority signals before general triage

## Design Principles

- **Vim keybindings as default** — `hjkl` navigation, `/` to filter, `?` for contextual help. Engineers who adopt TUIs have this muscle memory.
- **Single-key operations** — multi-step actions (open PR in browser, jump to group, mark reviewed) collapse to one keystroke. This is the TUI's core value over a CLI.
- **Multi-panel layout** — PR list and PR detail in sync; navigating the list updates the context pane without leaving the tool.
- **Speed as a constraint** — every interaction should be measurably faster than the alternative (browser tab-switching, `gh pr list`, Slack scanning). If it isn't, it isn't done.
- **Multi-provider first-class** — GitHub must be excellent from day one, but GitLab, Gitea, and Codeberg are not afterthoughts. Provider parity is a commitment, not a roadmap item.

## Tech Stack

Not decided. Options being considered:

- **OpenTUI** (Zig-based TUI framework) — interesting but Zig ecosystem maturity is unclear; needs research
- Go-based TUI libraries (Bubble Tea / Lip Gloss from Charm) — proven, good ecosystem
- Other options TBD

Research the OpenTUI/Zig ecosystem before committing to a stack.

## Non-Goals (v1)

- Inline review actions (approve, comment, merge) — prsm surfaces what to review; the web UI does the review
- Replacing the web UI or becoming a full GitHub/GitLab client
- Issue tracker support — architecture stays open to it, but out of v1 scope

## Open Questions

- Tech stack: Zig (OpenTUI) vs. Go (Bubble Tea) vs. other?
- Config format for provider credentials and view definitions
