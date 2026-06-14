# ADR-007: Event Engine and Hook System

## Status

Proposed

## Context

prsm polls provider APIs on a configurable interval (ADR-003) and maintains a normalized `PullRequest` snapshot in memory. Between any two poll cycles, the snapshot changes: PRs appear, disappear, change state, gain reviewers, receive commits. These transitions are signals — they are precisely what engineers need to know about — but the current architecture has no way to surface them as events.

Three consumer needs drive this:

1. **Notifications.** Engineers want to be alerted when something changes on a PR they care about: a review is requested, a new commit lands, a draft goes ready. These alerts currently require manually watching a browser or Slack thread.

2. **Automation.** User-configured shell commands should fire on state transitions — desktop notifications, Slack messages, custom scripts. This turns prsm into an event source for personal automation, not just a display tool.

3. **Library consumers.** When prsm is used as a Go library (a first-class planned consumer per ADR-000), calling code needs a way to subscribe to typed events rather than polling a returned slice. The event stream is what makes the library genuinely useful for reactive applications.

### Why not webhooks

ADR-003 already closes the door on inbound webhooks as a fetch mechanism: they require a publicly reachable URL, are GitHub-only for local forwarding, and impose per-repository registration burden. The Event Engine is not a replacement for webhooks — it is a different thing: an outbound event stream derived from prsm's own polling loop. No public URL, no provider registration, no daemon required beyond the prsm process itself.

### State persistence scope

Events fire while prsm is running. The in-memory state cache is established on first load and cleared on process exit. There is no cross-session persistence requirement — "what changed while prsm wasn't running" is out of scope. This keeps the Event Engine free of any storage dependency.

---

## Decision

### Layer placement

The Event Engine is a fifth architectural layer, running in parallel with the Query Layer (see ADR-000). It taps the normalized resource snapshot output from the Resource Model layer after each poll cycle, diffs it against an in-memory state cache, and emits typed events to a broadcast bus.

```
Resource Model output ([]PullRequest)
         │
    ┌────┴─────────────────────────┐
    │                              │
    ▼                              ▼
Query Layer                  Event Engine
(filter/sort/group)          (delta detect → Bus[Event[T]])
    │                              │
    └────────────┬─────────────────┘
                 ▼
            Consumers
   (TUI · MCP · HTTP API · library)
```

### Delta algorithm

After each poll cycle:

1. Receive the new `[]PullRequest` snapshot from the resource model.
2. Index by identity key: `(Provider.Kind, Provider.Host, ProviderID)`.
3. For each PR in the new snapshot:
   - If **not in cache** → emit `pr.appeared`; add to cache.
   - If **in cache** → diff specific fields; emit any transition events; update cache entry.
4. For each PR in the cache **not in the new snapshot** → emit `pr.disappeared`; remove from cache.
5. Publish all emitted events to the `Bus`.

Field diffs that produce events:

| Diff condition | Event |
|---|---|
| `State` changed from `PRStateDraft` to `PRStateOpen` or vice versa | `pr.draft_changed` |
| `State` changed to `PRStateMerged` | `pr.merged` |
| `State` changed to `PRStateClosed` | `pr.closed` |
| `State` changed to `PRStateOpen` from `PRStateClosed` | `pr.reopened` |
| `Reviews.RequestedReviewers` gained a new entry | `pr.review_requested` |
| `Reviews.ReviewerStates` (loaded) gained a new submission | `pr.review_submitted` |
| `Reviews.AggregateState` transitioned to `AggregateReviewApproved` | `pr.approval_granted` |
| `Reviews.AggregateState` transitioned to `AggregateReviewChangesRequested` | `pr.changes_requested` |
| `HeadSHA` changed (both states non-empty) | `pr.new_commit` |
| `CI.State` changed (both states `LoadStateLoaded`) | `pr.ci_status_changed` |

`LoadResult[T]` state transitions (Pending → Loaded) do not produce events. Only value changes in loaded fields do — a field becoming available is not a PR state change.

### First-load behavior

On startup the cache is empty. Without suppression, every PR in the first snapshot would emit `pr.appeared`, flooding subscribers with stale events. The default behavior is to **establish a baseline** on first load: populate the cache without emitting any events. Subsequent poll cycles emit normally.

This is configurable:

```toml
[events]
emit_on_first_load = false  # default: establish baseline, suppress first-load events
```

### Event types

```go
type EventKind string

const (
    EventPRAppeared         EventKind = "pr.appeared"
    EventPRDisappeared      EventKind = "pr.disappeared"
    EventPRReviewRequested  EventKind = "pr.review_requested"
    EventPRReviewSubmitted  EventKind = "pr.review_submitted"
    EventPRApprovalGranted  EventKind = "pr.approval_granted"
    EventPRChangesRequested EventKind = "pr.changes_requested"
    EventPRDraftChanged     EventKind = "pr.draft_changed"
    EventPRNewCommit        EventKind = "pr.new_commit"
    EventPRMerged           EventKind = "pr.merged"
    EventPRClosed           EventKind = "pr.closed"
    EventPRReopened         EventKind = "pr.reopened"
    EventPRCIStatusChanged  EventKind = "pr.ci_status_changed"
)

// Event[T] is a typed state-change notification.
// Previous is the zero value for EventPRAppeared (no prior state).
type Event[T any] struct {
    Kind      EventKind
    Timestamp time.Time
    Current   T
    Previous  T
}
```

### Broadcast bus

Go channels deliver to exactly one reader. With multiple subscribers (TUI notifications panel, shell hook runner, library consumers), fan-out requires a broadcaster. The implementation is a generic `Bus[T]` — approximately 50 lines, no external dependency:

```go
type Bus[T any] struct {
    mu   sync.Mutex
    subs []chan T
}

func (b *Bus[T]) Subscribe(buf int) <-chan T {
    ch := make(chan T, buf)
    b.mu.Lock()
    b.subs = append(b.subs, ch)
    b.mu.Unlock()
    return ch
}

func (b *Bus[T]) Publish(v T) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for _, ch := range b.subs {
        select {
        case ch <- v:
        default: // slow subscriber: drop
        }
    }
}

func (b *Bus[T]) Close() {
    b.mu.Lock()
    defer b.mu.Unlock()
    for _, ch := range b.subs {
        close(ch)
    }
    b.subs = nil
}
```

A slow or panicking subscriber cannot block delivery to others. Dropped events are not retried; the next poll cycle will reflect current state regardless.

### Public library API

Library consumers subscribe via a typed `EventStream` interface:

```go
// Subscribe returns a channel that receives events of the requested kinds.
// Passing no kinds subscribes to all events. The channel is closed when ctx
// is cancelled or the client shuts down.
func (s *EventStream[T]) Subscribe(ctx context.Context, kinds ...EventKind) <-chan Event[T]
```

Usage:

```go
client := prsm.NewClient(config)
ch := client.Events().Subscribe(ctx, prsm.EventPRReviewRequested, prsm.EventPRNewCommit)
for event := range ch {
    // event.Current and event.Previous are fully typed PullRequest values
}
```

### Shell hook configuration

User-defined shell hooks are configured in `config.toml`. Each hook specifies an event kind, an optional filter (using the same `FilterSpec` vocabulary as ADR-006), and a command template.

```toml
[events]
emit_on_first_load = false

[[hooks]]
event   = "pr.review_requested"
command = "osascript -e 'display notification \"{{.Current.Title}}\" with title \"prsm\"'"

[[hooks]]
event           = "pr.new_commit"
filter.reviewer = "me"
command         = "terminal-notifier -title prsm -message '{{.Current.Title}} has a new commit'"

[[hooks]]
event         = "pr.merged"
filter.author = "me"
command       = "say 'pull request merged'"
```

Template variables available in `command`:

| Variable | Value |
|---|---|
| `{{.Kind}}` | Event kind string, e.g., `pr.review_requested` |
| `{{.Timestamp}}` | RFC3339 timestamp |
| `{{.Current.*}}` | Any field on the current `PullRequest` |
| `{{.Previous.*}}` | Any field on the previous `PullRequest` (empty for `pr.appeared`) |

Hook commands are executed asynchronously via `exec.Command` in a separate goroutine. A failing hook (non-zero exit) is logged but does not affect other hooks or the poll loop. Hook output (stdout/stderr) is captured and surfaced in the TUI notifications panel for debugging.

### TUI notifications panel

The TUI subscribes to the event bus as one of its consumers. Received events are appended to a bounded in-memory ring buffer (default: 100 entries). A dedicated panel (toggled by `n`) displays this log with timestamps and event kinds. This serves as both a user-facing notification history and a debug surface for hook configuration.

---

## Consequences

### What this enables

- **Notifications**: desktop alerts, sound, or any shell-scriptable notification mechanism, without browser polling or Slack scanning.
- **Automation**: prsm becomes an event source for personal automation. Engineers can wire it to Slack bots, status bars, or custom dashboards via shell commands or the library API.
- **Library value**: the exported event stream is the primary reason to use prsm as a library rather than a CLI. It gives calling code a reactive, typed stream of PR state changes across all configured providers — something no single provider's API offers without per-provider webhook setup.
- **Cross-provider unification**: a single hook fires for `pr.review_requested` regardless of whether the PR is on GitHub, GitLab, or Gitea/Forgejo. prsm is the only tool positioned to provide this unified event stream.

### What this does not change

- The Resource Model, Query Layer, and Provider Adapters require no modification. The Event Engine is additive.
- ADR-003's polling architecture is unchanged. The Event Engine taps the existing poll cycle output; it does not add a new fetch loop.
- ADR-006's `FilterSpec` is reused for hook filters without extension.

### ADR-004 dependency

The `PullRequest` type requires a `HeadSHA string` field (the commit SHA at the tip of the source branch) to support `pr.new_commit` detection. This field is available in the list API response at all three v1 providers:

| Provider | Field |
|---|---|
| GitHub | `head.sha` |
| GitLab | `sha` |
| Gitea/Forgejo | `head.sha` |

`HeadSHA` has been added to the `PullRequest` struct definition in ADR-004.

### Incremental delivery

The Event Engine is planned for v1.1. v1 ships with the polling loop and TUI consumer only. No v1 code needs to anticipate the Event Engine at the implementation level — only the `HeadSHA` field in the resource model needs to be present from day one so that v1.1 can diff it without a schema migration.

### Hook reliability

Hooks are fire-and-forget. prsm does not retry failed hooks, queue them across restarts, or guarantee delivery if the process exits mid-execution. This is the correct default for a local triage tool — hooks are for convenience automation, not critical workflows. If reliability is required, library consumers can implement their own retry logic.
