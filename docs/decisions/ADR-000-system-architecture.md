# ADR-000: System Architecture

## Status

Accepted

## Context

prsm's stated goal is to give engineers a live, filterable view of everything waiting for their attention across multiple git hosting providers. Achieving this sustainably — across multiple resource types, multiple providers, and multiple consumers — requires a clear architectural layering that is established before implementation begins.

Two forces shape this decision:

1. **Multiple resource types.** Pull requests are the v1 resource. Issues are a planned future resource. The architecture must treat resource types as equal first-class citizens, not bolt-on extensions to a PR-centric system.

2. **Multiple consumers.** The TUI is prsm's first consumer, not its identity. The normalized, queryable resource data is prsm's real value. An MCP server, HTTP API, and reusable library are first-class planned consumers. The data layer must be built as if all four consumers exist simultaneously.

## Decision

prsm is organized into five layers with strict boundaries:

```
┌─────────────────────────────────────┐
│         Provider Adapters            │  GitHub, GitLab, Gitea/Forgejo
└──────────────────┬──────────────────┘
                   │  []ResourceType (PullRequest, Issue, …)
┌──────────────────▼──────────────────┐
│           Resource Model             │  Normalized types, LoadResult[T]
└──────────┬───────────────┬──────────┘
           │               │
┌──────────▼──────────┐  ┌─▼──────────────────────────────────┐
│     Query Layer      │  │          Event Engine               │
│                      │  │  Delta detection · Bus[T] · hooks   │
└──────────┬──────────┘  └─┬──────────────────────────────────┘
           │               │  []Event[T] (broadcast to subscribers)
           │  []ResourceType (filtered, sorted, grouped)
┌──────────▼───────────────▼──────────┐
│             Consumers                │  TUI · MCP server · HTTP API · library
└─────────────────────────────────────┘
```

### Layer 1: Provider Adapters

Each provider (GitHub, GitLab, Gitea/Forgejo) implements a `ProviderAdapter` interface. The adapter's job is narrow: authenticate, fetch raw API responses, and normalize them into the resource model. Adapters carry no presentation or query logic.

- One adapter per provider kind; a `ProviderInstance` config (host + credentials) instantiates a concrete adapter.
- Adapters return `[]PullRequest`, `[]Issue`, etc. — not raw API responses.
- Lazy-loaded fields (`LoadResult[T]`) are fetched by the adapter on demand, not eagerly.
- The adapter interface is the only place provider-specific knowledge lives. No other layer knows about GitHub vs. GitLab differences.

### Layer 2: Resource Model

The normalized Go types that all adapters produce and all consumers receive. Defined in ADR-004.

- `PullRequest`, `Issue`, and future resource types are independent structs. They share structural patterns (`ProviderInstance`, `LoadResult[T]`) but do not share a base type or interface — they are not polymorphic at this layer.
- `LoadResult[T]` (Pending / Loaded / Absent / Error) makes lazy-fetch lifecycle a first-class concern in the type system, not a nil-pointer convention.
- Resource types carry no presentation or transport concerns. Nothing in these types encodes how or where they will be rendered or served.

### Layer 3: Query Layer

The filter, sort, and group logic that operates on resource model types. Defined in ADR-006.

- `FilterSpec` is resource-typed: `PRFilterSpec` for pull requests, `IssueFilterSpec` for issues. A `BaseFilterSpec` holds universal fields (`Author`, `Label`, `Repo`, `Provider`, `StalenessGTE`, `State`). Resource-specific specs embed the base and add their own fields.

  > **Amended by ADR-010 §5.** The universal set is `Author`, `Repo`, `Provider`, `Label`, `StalenessGTE`, `TargetBranch`. `State` is **per-resource**, not universal — the field *name* is shared but the *value domain* is not: pull requests are `open | closed | merged`, issues would be `open | closed`. `TargetBranch` joins the universal set. The embed relationship above is unchanged.

- `Predicate[T any]` is a generic runtime type compiled from a `FilterSpec`. `PRFilterSpec.Compile()` → `Predicate[PullRequest]`. No union type or resource interface is needed at the predicate level.

  > **Amended by ADR-010 §§2, 6.** `Compile` also reports the lazy fields the spec depends on, so a consumer can complete them before applying the predicate, and it takes the identity map relocated from `query` to `model`: `PRFilterSpec.Compile(model.ResolvedIdentities)` → `(Predicate[PullRequest], []model.LazyField, error)`. The claim that no union type or resource interface is needed is unchanged.

- Sort and group keys are resource-scoped. Universal keys (`repo`, `provider`, `author`, `updated`, `created`, `staleness`) apply to all resource types. Resource-specific keys (`review_status` for PRs; `milestone` for Issues) are valid only for their declared resource type. Invalid keys for the declared resource type are a config load-time error.
- The query layer is consumer-agnostic. The TUI, MCP server, HTTP API, and library all use the same `FilterSpec` → `Predicate[T]` pipeline. No consumer gets a special query path.

### Layer 4: Event Engine

A delta-detection and pub/sub layer that runs alongside the query layer. After each poll cycle, the Event Engine diffs the incoming resource snapshot against its in-memory state cache, emits typed events for any transitions detected, and broadcasts them to all subscribers via a generic `Bus[T]`. Defined in ADR-007.

- Events are typed and carry both the current and previous resource state.
- The Bus uses a per-subscriber buffered channel pattern (`Bus[Event[T]]`) so no subscriber can block another.
- Subscribers include: the TUI notifications panel, the shell hook runner (user-configured commands), and library consumers.
- State is in-memory only — the cache is established on first load and cleared on exit. No persistence layer.
- Hook filter expressions reuse the same `FilterSpec` vocabulary as the query layer (ADR-006).

### Layer 5: Consumers

Consumers serve a specific interface over the shared assembly layer. The TUI is the first consumer.

- **TUI (Bubble Tea v2)** — interactive terminal UI. The TUI owns the rendering loop, keybinding handling, and panel layout. It does not own the data model or query logic. The TUI also subscribes to the Event Engine to power a notifications panel.
- **MCP server** — exposes prsm's resource data to AI agents via the Model Context Protocol. Same model and query layer as the TUI; different transport.
- **HTTP API** — serves normalized resource data over HTTP/JSON for dashboards, scripts, and third-party integrations.
- **Library** — exposes the provider adapters, resource model, query layer, and event stream as importable Go packages for third-party tools. Library consumers subscribe to the Event Engine directly via typed channels.

A single shared **assembly layer** — `package prsm` at the module root — constructs provider adapters from configuration, resolves provider identities, fans out fetches, aggregates partial failures, and drives the poll cycle. Each consumer adds only a transport or presentation layer over that assembly.

No consumer requires changes to the model, query layer, or adapters, and **no consumer may re-implement assembly behavior** — liveness, partial-failure, and identity semantics are specified once and shared, or prsm becomes several products wearing one name. See ADR-009 for the assembly layer's responsibilities and public surface.

### Named view definitions

Named views (defined in ADR-005/006) require a `resource` field that is **required** (no default). This is the config-layer manifestation of resource typing:

```toml
[[views]]
name     = "my-reviews"
resource = "pr"        # required; "pr" | "issue" | future resource types

[views.filter]
reviewer = "me"
state    = "open"
draft    = false
```

> **Amended by ADR-010 §1.** `state = "open"` was added to this example. It is not new syntax — `state` previously defaulted to `"open"` and could be omitted, and ADR-010 §1 drops that default so an omitted `state` now matches every state. Without the line, this view would silently span closed and merged PRs. Every example that relied on the old default changed meaning; the same correction applies in ADR-005 and in the shipped config at `config/defaults.go`.

The `resource` field is required — not optional with a default — because defaulting to `"pr"` would encode an assumption that pull requests are the canonical resource type. All resource types are equals. A missing `resource` field is a config load-time error with a clear message.

## Consequences

### What this enables

- **Provider parity without coupling**: adding a new provider (Bitbucket, Azure DevOps) means implementing one adapter. Nothing else changes.
- **Resource type parity**: adding Issues means defining `Issue`, `IssueFilterSpec`, and a set of Issue adapters. The query layer, consumers, and config format extend naturally.
- **Consumer parity**: the MCP server and HTTP API do not require a separate data pipeline. They compose the same layers the TUI uses.
- **Testability**: each layer can be tested in isolation. Adapter tests verify normalization. Query layer tests verify predicate correctness against in-memory model instances. Consumer tests verify assembly.

### Layer boundary rules

- Adapters must not import consumer packages, and must not import the config package (ADR-008).
- The resource model must not import adapter or consumer packages.
- The query layer must not import adapter or consumer packages.
- The Event Engine must not import consumer packages; it may import the resource model only.

  > **See ADR-010 §6.** This rule is unchanged. It is, together with Layer 4's reuse of the `FilterSpec` vocabulary in hook filter expressions, the reason ADR-010 §6 relocates `ResolvedIdentities` from `query` to `model`: the Event Engine will have to resolve `"me"` sentinels in hook filters and may not name a `query` type.

- The assembly layer may import adapters, model, query, event, and config. It must not import any consumer package, and must not import a UI or transport framework.
- The config package must not import adapter packages.
- Consumers may import all lower layers.
- No layer may import a higher layer (no upward dependencies).

### Incremental delivery

v1 ships with:
- One resource type: `PullRequest`
- One provider: GitHub (establishes the adapter interface pattern)
- One consumer: TUI

v1.1 adds the Event Engine and hook system (ADR-007). The resource model and query layer require no changes for this addition — the Event Engine slots in as a parallel path alongside the existing query pipeline.

Each subsequent release adds providers, resource types, or consumers independently. The architecture is designed so that adding any one of these does not require modifying the others.
