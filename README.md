# prsm

Engineers lose time to code resources scattered across vendors, accounts, and machines — pull requests on one host, CI runs on another, branches and worktrees on disk. prsm centralizes them into one live, filterable, keyboard-driven surface, and lets you act on them from there.

```mermaid
flowchart LR
    subgraph Sources["Scattered across vendors, accounts, machines"]
        GH["GitHub"]
        GL["GitLab"]
        GT["Gitea / Forgejo"]
        CI["CI systems"]
        Local["Local checkout"]
    end

    prsm(("prsm"))

    GH --> prsm
    GL --> prsm
    GT --> prsm
    CI --> prsm
    Local --> prsm

    prsm --> View["One live, filterable,<br/>keyboard-driven view"]
    View --> Actions["Merge · comment · label ·<br/>request review · rerun · close"]
```

v1 ships the pull-request slice — GitHub, GitLab, and Gitea/Forgejo aggregated into a single TUI. CI runs, branches, and worktrees are planned for later.

## Architecture

prsm is a vendor-agnostic resource aggregation layer with the TUI as its first consumer, not its product identity. Five layers with strict downward-only dependencies, plus a shared assembly layer between them and the consumers:

```mermaid
flowchart TD
    Adapters["Provider Adapters<br/>GitHub · GitLab · Gitea/Forgejo"]
    Model["Resource Model<br/>PullRequest, Issue, ..."]
    Query["Query Layer<br/>filter · sort · group"]
    Event["Event Engine<br/>diff snapshots -> events -> hooks"]
    Assembly["Assembly (package prsm)<br/>adapters, identity, poll cycle"]
    Consumers["Consumers<br/>TUI · MCP server · HTTP API · library"]

    Adapters --> Model
    Model --> Query
    Model --> Event
    Query --> Assembly
    Event --> Assembly
    Assembly --> Consumers
```

See [`CLAUDE.md`](CLAUDE.md) for the full project overview.
