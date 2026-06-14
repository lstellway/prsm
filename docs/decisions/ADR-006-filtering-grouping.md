# ADR-006: Filtering and Grouping

## Status

Proposed

## Context

prsm's value proposition is triage: surfacing the right PRs at the right moment. Filtering, sorting, and grouping are the primary mechanisms through which engineers express "what needs my attention right now." This ADR defines the complete v1 filtering model — what fields are filterable, how filter predicates compose, how groupings work, what the `/` filter UX looks like, and how named views (defined in ADR-005) interact with session-level filtering.

### The data model (ADR-004 summary)

All filter and grouping expressions operate on the normalized `PullRequest` struct. Fields fall into two availability tiers:

| Field | Type | Available at | Notes |
|---|---|---|---|
| `Title` | string | list-time | Title text for fuzzy match |
| `Number` | int | list-time | PR/MR number |
| `State` | PRState enum | list-time | `open`, `closed`, `merged`, `draft` |
| `Draft` | bool | list-time | Redundant with `PRStateDraft` but retained for convenience |
| `Author.Username` | string | list-time | Provider login; supports `"me"` sentinel |
| `Author.DisplayName` | string | list-time | Falls back to Username if unavailable |
| `TargetBranch` | string | list-time | Base branch being merged into |
| `SourceBranch` | string | list-time | Head branch being merged from |
| `CreatedAt` | time.Time | list-time | Absolute creation timestamp |
| `UpdatedAt` | time.Time | list-time | Last activity timestamp |
| `Labels` | []Label | list-time | Name + color per label |
| `Reviews.RequestedReviewers` | []ReviewerState | list-time | Identities of requested reviewers; supports `"me"` sentinel |
| `Reviews.AggregateState` | AggregateReviewState enum | list-time (partial) | Derived from RequestedReviewers before ReviewerStates loads; becomes more accurate after lazy load |
| `Provider` | ProviderInstance | list-time | Kind, Host, Account — maps to provider name in config |
| `Repo` | Repository | list-time | Owner + Name |
| `CommentCount` | int | list-time | General comment count |
| `Mergeable` | MergeableState | list-time | `mergeable`, `conflicting`, `unknown` |
| `CI` | LoadResult[CIStatus] | **lazy** | Pending until secondary fetch completes; GitLab populates from list response |
| `Reviews.ReviewerStates` | LoadResult[[]ReviewerState] | **lazy** | Full individual reviewer decisions after secondary fetch |
| `Diff` | LoadResult[DiffStats] | **lazy** | Additions, deletions, changed files — requires detail endpoint |

Derived fields available at filter time (computed from list-time fields, no extra API calls):
- **staleness** — `time.Since(UpdatedAt)`, expressed as days
- **age** — `time.Since(CreatedAt)`, expressed as days

### What engineers actually do during triage

Observed triage workflows map to a small set of high-frequency queries:

1. **"What needs my review?"** — `reviewer=me` + `review_status=review_required` + `draft=false`
2. **"What are my open PRs?"** — `author=me` + `state=open`
3. **"What's stale?"** — `reviewer=me` + `staleness_days>=3`
4. **"What's blocking a merge?"** — `ci_status=failing` OR `review_status=changes_requested`
5. **"What's ready to land?"** — `review_status=approved` + `ci_status=passing`
6. **"No drafts"** — `draft=false` (usually combined with other filters)
7. **"This repo only"** — `repo=owner/name`

Use cases 1, 2, and 3 are addressable with AND-only composition and list-time fields. Use cases 4 and 5 require CI status (a lazy field). Use case 5 is the most filter-composition-heavy case in v1.

### Comparable tools

**k9s** uses `/` for live text filtering with multiple sub-modes: plain regex match on visible text (`/<term>`), negation (`/!<term>`), label selector (`/-l key=value`), and fuzzy (`/-f term`). The sub-mode prefix (`-l`, `-f`, `!`) extends the basic slash without requiring a separate UI control. Filters are session-local and ephemeral — they are cleared on `Escape`. k9s does not have named saved views with filters; the equivalent is "aliases" which change what resource type is shown, not filter parameters over a single resource type.

**lazygit** uses `/` uniformly across all panels to activate a live narrowing filter. In some views it narrows (hides non-matching rows); in others (commits) it highlights without hiding. Filters are cleared on `Escape`. lazygit also has a `Ctrl+S` path-filter mode for the commits view — a structured, context-aware filter that is distinct from the freeform `/` text filter. lazygit documents the distinction as "filtering" (hides rows) vs. "searching" (highlights rows, all items stay visible).

**Superhuman's split inbox** is the best reference for prsm's named view model. Splits are permanent, pre-defined rule-based segments that persist across sessions and appear as navigable sections within the inbox. They are not search results — they are structural: each split is a named bucket that the user cycles through intentionally. An empty split disappears. The key insight is that splits and ad-hoc search are two different UX layers: splits define "what I care about across sessions" while search answers "find this specific thing right now." Named views in prsm map directly to splits; the `/` session filter maps directly to ad-hoc search.

**blippy** (Rust/Ratatui, GitHub-only) uses `/` in its repository picker to narrow the repo list and supports GitHub-style qualifier syntax for issue/PR search (`is:open`, `is:closed`, `is:merged`, `label:<name>`, `assignee:<user>`). Blippy uses `Tab`/`Shift+Tab` to toggle between open and closed status and `1`/`2` as shortcuts. It does not appear to have named saved views or groupings — it relies on the qualifier syntax for all filtering.

### The lazy-field filtering problem

CI status (`CI`) and individual reviewer decisions (`Reviews.ReviewerStates`) are lazily loaded. When a user activates a filter on `ci_status=failing` and the CI data for some PRs is still `LoadStatePending`, there are four possible behaviors:

**Option A — Exclude pending items (false negatives):** PRs with unloaded CI data are hidden until the data arrives. Risk: the list appears incomplete during load; PRs may "pop in" after a few seconds.

**Option B — Include pending items (false positives):** PRs with unloaded CI data are shown regardless. Risk: non-matching PRs appear in a filtered view, creating noise.

**Option C — Show a distinct pending row:** Pending PRs appear in the list with a spinner or placeholder, clearly marked as "CI loading." Risk: UX complexity; requires special rendering for in-list loading states.

**Option D — Eager-load on filter activation:** When a lazy-field filter is activated (either via a named view on startup or via `/` in session), prsm prioritizes fetching that field for all currently-visible PRs before rendering the filtered list. Risk: adds latency to filter activation; may consume rate-limit budget.

### Filter composition models

**AND-only:** All predicates must pass. This covers the vast majority of triage use cases (e.g., `reviewer=me AND draft=false AND staleness>=3`). Simple to implement, simple to serialize to TOML, simple to explain.

**AND + OR:** Allows expressing "failing CI OR changes requested." More powerful, but requires a query tree representation (not a flat TOML table), increases UX complexity, and benefits a minority of triage queries in v1.

**Named segments (Superhuman-style):** Pre-defined view presets that are activated by name, not composed ad-hoc. The UX cost of AND-only is mitigated by having a library of named views that encode the common OR-requiring queries as separate view definitions ("my-reviews" and "ci-failures" as two views rather than one `OR`-containing filter).

### The `"me"` sentinel

ADR-005 established `reviewer = "me"` and `author = "me"` as config values that resolve to the authenticated user per provider. The resolution question is: when does `"me"` resolve to a concrete username?

At fetch time, prsm must already know the authenticated identity per provider to construct API queries (e.g., `GET /pulls?requested_reviewer=<username>`). This means the authenticated username is a required startup precondition for any provider that will be queried with `reviewer=me` or `author=me`. prsm must resolve identities at startup, not at filter evaluation time.

At filter evaluation time, `"me"` has already been resolved to a concrete `ProviderIdentity{Username: "loganstellway", Host: "github.com"}` during startup. The in-memory filter predicate compares `pr.Author.Username == resolvedMe.Username` (for that PR's provider), not the literal string `"me"`. This means a PR list containing PRs from multiple providers can apply a single `author=me` filter correctly because the resolved identity is per-provider.

---

## Decision

### 1. Essential v1 filters

The following filter fields are supported in v1, all evaluated against list-time data (no lazy dependency):

| Filter key | Type | Evaluates against | Notes |
|---|---|---|---|
| `author` | string or `"me"` | `Author.Username` | `"me"` resolved per provider at startup |
| `reviewer` | string or `"me"` | `Reviews.RequestedReviewers[].Username` | Matches if any requested reviewer matches |
| `review_status` | string enum | `Reviews.AggregateState` | `"approved"`, `"changes_requested"`, `"review_required"`, `"commented"`, `"none"` |
| `state` | string enum | `State` | `"open"`, `"closed"`, `"merged"`, `"draft"` — default `"open"` |
| `draft` | bool | `Draft` | `false` excludes drafts; `true` includes only drafts |
| `label` | string or []string | `Labels[].Name` | AND-match: PR must carry all listed labels |
| `repo` | string or []string | `Repo.Owner + "/" + Repo.Name` | `"owner/repo"` format; OR-match across list |
| `provider` | string or []string | `Provider.Account` name from config | Matches the provider's configured `name` field |
| `staleness_days` | int | `time.Since(UpdatedAt)` in days | Minimum age since last update (`>=` N days) |
| `target_branch` | string | `TargetBranch` | Substring match; useful for `"main"` or `"release/*"` |
| `ci_status` | string enum | `CI.State` (lazy) | `"passing"`, `"failing"`, `"pending"`, `"none"` — see lazy-field behavior below |

**Deferred to v2:**
- `source_branch` filter — low frequency; `target_branch` covers the primary use case (filter by base branch)
- `mergeable` filter — useful but most triage is review-centric; low priority for v1
- `comment_count` threshold — edge case; better served by sorting
- `diff_size` threshold (additions/deletions) — requires lazy `Diff` load; low priority for v1
- `age_days` (based on `CreatedAt`) — staleness covers the dominant use case; `created` sort addresses the rest

### 2. Lazy-field filter behavior: Option C (pending marker)

When a filter includes `ci_status` and a PR's `CI` field is `LoadStatePending`:
- The PR remains visible in the list with a `•` or spinner indicator in the CI column.
- The row is visually distinguishable from PRs that pass or fail the filter.
- Once the CI data loads, the row is immediately re-evaluated: PRs that do not match the filter are removed; PRs that match update their display to show the actual CI state.
- `LoadStateAbsent` (provider does not expose CI for this PR) is treated as `"none"` — it matches `ci_status="none"` and does not match any other value.
- `LoadStateError` is treated as `"none"` for filter evaluation; a visual error indicator is shown in the CI column.

This approach avoids false positives (Option B) and false negatives (Option A) during the load window. The "pop out" behavior (items disappearing when data arrives) is less disorienting than "pop in" because the item was already visible.

The same behavior applies to `review_status` filtering when `Reviews.ReviewerStates` is `LoadStatePending`: the `AggregateState` derived from `RequestedReviewers` alone is used as the initial evaluation, with re-evaluation once the full reviewer states load. This works well because the RequestedReviewers-derived AggregateState is a conservative lower bound — it reliably identifies `"review_required"` PRs, which is the most important triage case.

### 3. Filter composition: AND-only in v1

All v1 filter predicates compose with AND. A PR must satisfy every specified filter to appear in the view.

The AND-only model covers the six most important triage queries identified above. The OR use case ("failing CI OR changes requested") is addressed by providing two separate named views rather than a single OR-composed filter. This is the Superhuman model: named segments replace the need for complex boolean composition at the cost of slightly more config verbosity.

v2 may introduce OR composition via a `[[filter.any]]` / `[[filter.all]]` table structure in TOML. The internal predicate type is designed to support OR from the start (see Consequences).

**TOML filter syntax (consistent with ADR-005):**

```toml
[[views]]
name     = "my-reviews"
resource = "pr"   # required on every view; scopes valid filter/sort/group keys

[views.filter]
author         = "me"
draft          = false      # PR-specific field
staleness_days = 3
state          = "open"     # default; can omit

# Label AND: PR must have both labels
label          = ["needs-review", "priority-high"]

# Repo OR: PR may be in any of these repos
repo           = ["acme/api", "acme/frontend"]

# review_status and ci_status are PR-specific fields
review_status  = "review_required"
ci_status      = "passing"
```

Multiple values for `label` require the PR to carry **all** listed labels (AND). Multiple values for `repo` and `provider` match if the PR is in **any** of the listed values (OR). This asymmetry is intentional and matches how these fields are used in practice: label combinations are "must have both"; repo/provider lists are "any of these."

### 4. Sorting

Four sort orders are supported in v1:

| Sort key | Direction | Evaluates against | Notes |
|---|---|---|---|
| `updated` | desc (default) | `UpdatedAt` | Most recently touched first |
| `created` | desc | `CreatedAt` | Newest PRs first |
| `staleness` | asc | `UpdatedAt` | Least recently touched first (oldest-first triage) |
| `title` | asc | `Title` | Alphabetical |

**TOML syntax:**

```toml
[views.sort]
by        = "updated"   # "updated" | "created" | "staleness" | "title"
direction = "desc"      # "asc" | "desc"
```

Default sort when no sort is specified: `updated desc`.

PRs with lazy fields still pending are sorted-last within their sort tier (e.g., when sorting by staleness, pending-CI PRs sort the same as any other — staleness does not depend on CI data).

### 5. Grouping

At most one grouping is active at a time. Nested groupings (e.g., group by provider, then by repo) are deferred to v2. The complexity of nested group headers in a Bubble Tea list model is non-trivial and the single-level groupings cover the primary use cases.

**Supported groupings in v1:**

| Group key | Scope | Groups by | Use case |
|---|---|---|---|
| `none` | universal | No grouping; flat list | Single-provider or homogeneous repo setups |
| `repo` | universal | `Repo.Owner + "/" + Repo.Name` | Most common: see all PRs per project |
| `provider` | universal | `Provider.Account` name from config | Multi-provider users who want org-level separation |
| `author` | universal | `Author.Username` | Team leads reviewing the team's output |
| `review_status` | PR only | `Reviews.AggregateState` | Triage-by-stage: "needs review" section first, "approved" section last |

Universal grouping keys are valid for all resource types. PR-only keys (`review_status`) are valid only when the view's `resource = "pr"` — using them on an Issue view is a config load-time error.

**Group ordering:**
- `repo` and `provider` groups are sorted alphabetically by group key.
- `author` groups are sorted by PR count descending (most active author first).
- `review_status` groups follow triage priority order: `review_required` → `changes_requested` → `commented` → `approved` → `none`. This order reflects urgency: what needs attention appears at the top.

**TOML syntax:**

```toml
[views.group]
# Universal keys: "none" | "repo" | "provider" | "author"
# PR-only keys:   "review_status"
by = "repo"
```

Groups with zero matching PRs are not rendered (empty group headers are never shown).

### 6. The `/` session filter UX

`/` activates a live session filter. The session filter is ephemeral — it exists only during the current prsm session and is not persisted to config. It is distinct from named views (which are persisted presets).

**Behavior:**

- Pressing `/` opens an input bar at the bottom of the PR list panel (consistent with k9s, lazygit, and vim).
- Typing begins immediately; the list narrows in real time as each character is entered.
- The session filter is a **fuzzy text match** against a composite string of: `Title`, `Author.Username`, `Author.DisplayName`, `Repo.Owner`, `Repo.Name`, `Labels[].Name`, `SourceBranch`, `TargetBranch`. Fuzzy match uses a standard scoring algorithm (e.g., fzf-style: consecutive character bonuses, start-of-word bonuses).
- The fuzzy match operates on top of the active named view's structural filter. Example: if the active view filters `reviewer=me`, pressing `/` and typing `frontend` fuzzy-matches against that already-filtered set.
- Pressing `Escape` clears the session filter and restores the named view's full result set. The named view remains active.
- The filter bar shows the current query string and a match count: `/ frontend  [4/23]`.
- There is no structured qualifier syntax in the session filter (no `author:me` style). The `/` bar is fast fuzzy-text-only. Users who want structured filtering define named views in their config.

**Rationale for fuzzy-text-only `/`:**

k9s extends `/` with `-l` and `-f` prefixes, which is ergonomic for Kubernetes label selectors but creates a syntax the user must learn. lazygit keeps `/` as simple text search across all panels. blippy uses GitHub qualifier syntax (`is:open`, `label:foo`) but this requires learning a new query language. prsm's structured filtering is already expressed in named views (config-time, not session-time). The session-filter bar's job is speed, not power — engineers use it to find a specific PR by title or repo name quickly. Named views handle the power use case.

### 7. Named views and session filter interaction

Named views and the session filter are two independent layers that compose:

```
named view filter (structural, persisted)
    └── session filter (fuzzy text, ephemeral)
        └── rendered PR list
```

**Mental model:** A named view is a lens (what kind of PRs to focus on); the session filter is a spotlight within that lens (find a specific PR fast).

**UX rules:**
- Activating a named view replaces the currently active named view's filter. The session filter (`/` query) is **cleared** when switching views. This prevents stale session filters from hiding items in the new view.
- A named view is activated by pressing `v` (or configurable key) to open the view picker, then selecting a view by name. The view name is displayed in the prsm header.
- The `[defaults]` TOML table sets which named view is active on startup (`default_view`).
- If no named view is active (e.g., after pressing `Escape` on the view picker or on first run without a `default_view`), prsm shows the full unfiltered PR set across all configured providers, sorted by `updated desc`. This is the "all PRs" baseline.

**Session filter does not write to config.** Users who find themselves repeatedly applying the same text filter should create a named view.

---

## Consequences

### Go predicate type

The internal filter representation uses a struct-based predicate tree rather than raw `func(PullRequest) bool` values. This is required for two reasons: (1) struct-based predicates are serializable to/from TOML config, enabling round-trip config loading; (2) struct-based predicates can be composed programmatically (the view picker and future query builder can construct predicates without string parsing).

Sketch of the predicate types:

```go
// FilterSpec is the serializable filter specification stored in config and
// constructed by the view picker. It is compiled to a Predicate[PullRequest]
// at startup (or when the filter changes).
type FilterSpec struct {
    Author        string   // "" = no filter; "me" = authenticated user
    Reviewer      string   // "" = no filter; "me" = authenticated user
    ReviewStatus  string   // AggregateReviewState value or ""
    State         string   // PRState value or ""
    Draft         *bool    // nil = no filter; &true = drafts only; &false = no drafts
    Label         []string // AND-match: PR must carry all listed labels
    Repo          []string // OR-match: PR must be in one of these repos
    Provider      []string // OR-match: PR must be from one of these providers
    CIStatus      string   // CIState value or ""
    StalenessGTE  int      // >= N days since UpdatedAt; 0 = no filter
    TargetBranch  string   // substring match; "" = no filter
}

// Predicate[T] is the runtime evaluation type.
// Composed via And/Or; evaluated against each PullRequest during list rendering.
type Predicate[T any] func(T) bool

func (p Predicate[T]) And(other Predicate[T]) Predicate[T] {
    return func(v T) bool { return p(v) && other(v) }
}

// FilterSpec.Compile(resolvedMe map[ProviderKind]string) Predicate[PullRequest]
// converts the serialized spec into a runtime predicate, resolving "me" sentinels.
```

`FilterSpec` maps 1:1 to the `[views.filter]` TOML table. The `Compile` method produces a `Predicate[PullRequest]` that is applied by the Bubble Tea Update function when the PR list is refreshed.

### Bubble Tea integration

The TUI model holds:
- `activeView string` — the name of the current named view (or `""` for "all PRs")
- `compiledFilter Predicate[PullRequest]` — compiled once when the view activates or when `resolvedMe` becomes available at startup
- `sessionQuery string` — the current `/` filter input
- `displayList []PullRequest` — the result of applying `compiledFilter` then fuzzy-matching `sessionQuery` against the full in-memory PR list

When a tea.Msg arrives carrying new or updated PR data (e.g., a secondary fetch completing CI status), the update function re-runs `compiledFilter` and `sessionQuery` over the updated item. Items that newly pass the filter are inserted into `displayList`; items that newly fail are removed. This is the "re-evaluation on data arrival" behavior for lazy-field filters.

The fuzzy match for `/` is computed on-the-fly for each keystroke. The composite target string per PR is precomputed and cached in the model (`fuzzyIndex map[string]string` keyed by `ProviderID`) to avoid string concatenation on every keystroke.

### Startup: resolving `"me"` sentinels

At startup, before the TUI renders, prsm makes a `GET /user` (GitHub) / `GET /api/v4/user` (GitLab) / `GET /api/v1/user` (Gitea) call per configured provider to resolve the authenticated user's identity. This call is cheap (< 1 KB response, no pagination) and is made concurrently with the initial PR list fetch. If the identity call fails for a provider, filters using `"me"` for that provider are disabled with a startup warning; other providers continue normally.

The resolved identities are stored in the model as `map[ProviderKind]ResolvedIdentity` and passed to `FilterSpec.Compile`.

### Performance: lazy-field re-evaluation

For a list of N PRs, re-evaluating the compiled filter on every lazy-field update is O(1) per update (only the updated PR is re-evaluated, not the full list). The `displayList` is maintained as an ordered slice with a parallel `visibleSet map[string]bool` keyed by `ProviderID` to allow O(1) insert/remove without full re-scan. This makes lazy-field filter updates non-blocking and fast even for large lists.

### What is deferred to v2

- **OR composition within a single view** — `[[filter.any]]` table syntax expressing OR predicates
- **Nested groupings** — group by provider, then by repo within each provider section
- **`/` structured qualifiers** — `reviewer:me`, `label:bug` syntax in the session filter bar (after validation that users want it)
- **Sort by review status or CI status** — deferred because both involve lazy fields; sorting on them before data loads produces unstable orderings
- **`source_branch`, `mergeable`, `diff_size`, `age_days` filters** — low frequency in v1 triage scenarios
- **Saved session filters** — ability to promote a `/` query to a named view from within the TUI
- **Filter history** — re-applying recent session filters (similar to shell history in the filter bar)
