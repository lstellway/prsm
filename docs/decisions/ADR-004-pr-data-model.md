# ADR-004: Generic PR Data Model

## Status

Proposed

## Context

prsm normalizes pull request and merge request data from three v1 providers (GitHub, GitLab, Gitea/Forgejo) into a single internal schema. All views, filters, sorts, and groupings operate on this normalized type — no provider-specific types leak into the display or triage layer.

### The normalization problem

Each provider exposes different terminology (GitHub "pull requests" vs. GitLab "merge requests"), different field availability per API response, and different review state vocabularies. The normalized schema must be rich enough to drive triage-oriented views while not requiring every field to be populated for every provider.

### Per-provider field availability at list level

| Field | GitHub REST list | GitLab REST list | Gitea/Forgejo REST list |
|---|---|---|---|
| `title` | ✓ | ✓ | ✓ |
| `draft` | ✓ | ✓ | ✓ |
| `created_at` | ✓ | ✓ | ✓ |
| `updated_at` | ✓ | ✓ | ✓ |
| `labels` | ✓ (name+color) | ✓ (name+color+id+description) | ✓ (name+color) |
| `requested_reviewers` | ✓ (identity only) | ✓ via `reviewers[]` | ✗ (not in list) |
| `review states` | ✗ (separate `/reviews` call or GraphQL) | ✗ (approval state needs `/approvals`) | ✗ (separate `/reviews` call) |
| `CI/check status` | ✗ (separate `/check-runs` call) | ✓ inline via `head_pipeline` | ✗ (separate Gitea Actions call) |
| `mergeable` | ✓ (`mergeable`, `mergeable_state`) | ✓ (`detailed_merge_status`) | ✓ (`mergeable`) |
| `comments count` | ✓ | ✓ (`user_notes_count`) | ✓ (`comments`) |
| `commits count` | ✗ (detail endpoint) | ✗ (detail endpoint) | ✗ (detail endpoint) |
| `changed files count` | ✗ (detail endpoint) | ✗ (detail endpoint) | ✗ (detail endpoint) |

### The lazy-load challenge

Three field categories cannot be reliably populated from the list API response across all providers:

1. **Review states** — individual reviewer decisions (approved, changes requested, etc.) require a secondary call at all three providers. The list response may include reviewer identities (who is requested to review) but not whether they have submitted a review and what that review said.

2. **CI status** — GitLab includes pipeline status inline; GitHub and Gitea/Forgejo require a separate call. Treating CI as always-available would force extra round-trips for GitHub and Gitea users on every list refresh, consuming rate-limit budget unnecessarily.

3. **Diff counts** — commits, changed files, additions, deletions. These require the individual PR detail endpoint at all three providers.

### The "not loaded yet" vs. "not present" distinction

Raw `*T` pointer fields are ambiguous: `nil` means "this field was not included in the struct initialization" or "this field is absent from the provider" or "this field has not been fetched yet." For a TUI that lazily loads secondary data, this ambiguity makes it impossible to know whether to show a spinner, show an absence indicator, or show a provider-capability gap.

The chosen approach is a generic `LoadResult[T]` type with three states: **Pending** (not yet fetched), **Loaded** (fetched, has a value), and **Absent** (provider does not expose this field). This is preferred over raw pointers because it explicitly models fetch lifecycle alongside nullability.

### Review state vocabulary across providers

| State | GitHub | GitLab | Gitea/Forgejo |
|---|---|---|---|
| Approved | `APPROVED` | approved event | `APPROVED` |
| Changes requested | `CHANGES_REQUESTED` | (no direct equivalent; maps to "unapproved" or blocking) | `REQUEST_CHANGES` |
| Commented only | `COMMENTED` | `reviewed` (state on reviewer object) | `COMMENT` |
| Dismissed | `DISMISSED` | (no direct equivalent) | ✗ |
| Pending / not yet reviewed | `PENDING` | `unreviewed` (state on reviewer object) | ✗ |

The normalized `ReviewDecision` enum covers the union of meaningful states: `Approved`, `ChangesRequested`, `Commented`, `Dismissed`, `Pending`. Provider adapters map provider-specific strings to this enum; unmappable states map to `Pending` as the safest default.

### Aggregate review state

The list view needs a single triage signal per PR — "needs my review," "approved," "blocked by changes requested." This requires aggregating across all individual review decisions. The `ReviewSummary.AggregateState` field captures this. The aggregation rules are:

- `ChangesRequested` wins if any reviewer requested changes.
- `Approved` wins if all required reviews are approved and none have unresolved change requests.
- `ReviewRequired` if there are outstanding requested reviewers who have not yet submitted any decision.
- `Commented` if reviews exist but are all comment-only.
- `None` if no review activity at all.

### Self-hosted instances

Users may have both `github.com` and `github.example.com` configured simultaneously. The `ProviderInstance` struct captures the canonical hostname per configured provider, allowing the display layer to show which instance a PR belongs to and allowing adapters to use the correct base URL for API calls.

### Score / urgency field

A computed triage score was considered as a top-level field and rejected. Priority is not a property of a PR — it is a property of a view configuration. Users express priority through sort and filter expressions (e.g., sort by `changed_files` ASC, then `created_at` DESC). The data model provides the raw inputs; the view layer composes them into order. A baked-in score would collapse multiple signals into one opaque number, couple the model to a specific weighting, and make priority invisible to the user.

### Label model

GitHub provides name and hex color. GitLab provides name, color, description, and numeric ID. Gitea/Forgejo provides name and color. The normalized `Label` type uses the lowest common denominator: name and color. The color is stored as a hex string (e.g., `"#0075ca"`). GitLab's description and ID are dropped at normalization time (available if the raw provider data is retained, but not surfaced in v1 views).

### Comparable tools reviewed

- `blippy` (Rust/Ratatui) — keyboard-first GitHub PR TUI; uses Rust enums with `Option<T>` for nullable fields, which maps cleanly to Go's `*T` or the `LoadResult[T]` pattern. Its schema is GitHub-only and does not need multi-provider normalization.
- `github_types` Rust crate — uses `Option<T>` for all nullable fields on the provider-native `PullRequest` struct. The prsm normalized type follows the same principle but distinguishes "provider absent" from "not yet loaded."
- `google/go-github` — represents `State` and review states as `*string`, using pointer-as-optional throughout. prsm intentionally avoids raw `*string` for semantic states, preferring typed enums to catch mapping errors at compile time.
- `edgedb-go` `Optional{T}` pattern — explicit `isSet bool` flag alongside the value; the inspiration for `LoadResult[T]`.
- k9s — uses Go interfaces for resource kinds, with each resource type implementing a `Render` interface. The provider-agnostic rendering layer is the direct analog to prsm's normalized schema feeding provider-agnostic views.

---

## Decision

The normalized PR data model uses the following Go types.

### Core types

```go
// ProviderKind identifies the git hosting software, independent of the instance.
type ProviderKind string

const (
    ProviderGitHub ProviderKind = "github"
    ProviderGitLab ProviderKind = "gitlab"
    ProviderGitea  ProviderKind = "gitea" // covers Gitea, Forgejo, Codeberg
)

// ProviderInstance identifies one configured account/server combination.
// Multiple instances of the same provider kind are supported (e.g., github.com
// and github.example.com as two separate entries).
type ProviderInstance struct {
    Kind    ProviderKind // "github", "gitlab", "gitea"
    Host    string       // canonical hostname, e.g., "github.com" or "gitlab.example.com"
    Account string       // username or org slug used for this credential
}

// LoadResult[T] represents a field that may not yet have been fetched, may have
// a value, or may be absent because the provider does not supply it.
//
// The zero value represents LoadStatePending (not yet fetched).
type LoadResult[T any] struct {
    state LoadState
    value T
}

type LoadState uint8

const (
    LoadStatePending LoadState = iota // zero value: not yet fetched
    LoadStateLoaded                   // fetched successfully; value is valid
    LoadStateAbsent                   // provider does not expose this field
    LoadStateError                    // fetch was attempted but failed
)

func Pending[T any]() LoadResult[T]           { return LoadResult[T]{state: LoadStatePending} }
func Loaded[T any](v T) LoadResult[T]         { return LoadResult[T]{state: LoadStateLoaded, value: v} }
func Absent[T any]() LoadResult[T]            { return LoadResult[T]{state: LoadStateAbsent} }

func (r LoadResult[T]) IsLoaded() bool        { return r.state == LoadStateLoaded }
func (r LoadResult[T]) IsPending() bool       { return r.state == LoadStatePending }
func (r LoadResult[T]) IsAbsent() bool        { return r.state == LoadStateAbsent }
func (r LoadResult[T]) Get() (T, bool)        { return r.value, r.state == LoadStateLoaded }
func (r LoadResult[T]) MustGet() T            {
    if r.state != LoadStateLoaded {
        panic("LoadResult.MustGet called on non-Loaded result")
    }
    return r.value
}
func (r LoadResult[T]) UnwrapOr(def T) T {
    if r.state == LoadStateLoaded {
        return r.value
    }
    return def
}
```

### PR state enums

```go
// PRState is the lifecycle state of the PR.
type PRState string

const (
    PRStateOpen   PRState = "open"
    PRStateClosed PRState = "closed"
    PRStateMerged PRState = "merged"
    PRStateDraft  PRState = "draft" // open + draft; draft is surfaced separately for triage
)

// ReviewDecision is one reviewer's verdict on a PR.
// Maps the union of GitHub, GitLab, and Gitea review state strings.
type ReviewDecision string

const (
    ReviewDecisionApproved         ReviewDecision = "approved"
    ReviewDecisionChangesRequested ReviewDecision = "changes_requested"
    ReviewDecisionCommented        ReviewDecision = "commented"
    ReviewDecisionDismissed        ReviewDecision = "dismissed"
    ReviewDecisionPending          ReviewDecision = "pending" // requested but no decision yet
)

// AggregateReviewState is the rolled-up review verdict for display and triage sorting.
type AggregateReviewState string

const (
    AggregateReviewNone             AggregateReviewState = "none"               // no reviewers requested or assigned
    AggregateReviewRequired         AggregateReviewState = "review_required"    // reviewers assigned, none submitted
    AggregateReviewApproved         AggregateReviewState = "approved"           // all required approvals met
    AggregateReviewChangesRequested AggregateReviewState = "changes_requested"  // at least one change request outstanding
    AggregateReviewCommented        AggregateReviewState = "commented"          // reviews exist but all are comment-only
)

// CIState is the overall status of CI/check runs for the PR head commit.
type CIState string

const (
    CIStatePassing CIState = "passing"
    CIStateFailing CIState = "failing"
    CIStatePending CIState = "pending"
    CIStateNone    CIState = "none" // no CI configured or checks not found
)

// MergeableState is the mergeable state of the PR.
type MergeableState string

const (
    MergeableStateMergeable   MergeableState = "mergeable"
    MergeableStateConflicting MergeableState = "conflicting"
    MergeableStateUnknown     MergeableState = "unknown" // not yet computed by provider
)
```

### Supporting types

```go
// Author is the PR author's normalized identity.
type Author struct {
    Username    string  // provider login/username
    DisplayName string  // may equal Username if display name is unavailable
    AvatarURL   string  // empty string if not available
}

// Label is a normalized tag attached to a PR.
type Label struct {
    Name  string // display name
    Color string // hex color, e.g., "#0075ca"; empty if provider does not supply one
}

// Repository identifies the repo the PR belongs to.
type Repository struct {
    Owner string // org or user namespace
    Name  string // repository name (without owner prefix)
}

// ReviewerState captures one reviewer's identity and their current decision.
type ReviewerState struct {
    Username string
    Decision ReviewDecision
}

// ReviewSummary is the full review picture for a PR.
// LoadResult wraps it because individual reviewer states require a secondary fetch.
type ReviewSummary struct {
    // RequestedReviewers is populated from the list API response at all providers
    // that return it. It contains reviewer identities with a Pending decision.
    RequestedReviewers []ReviewerState

    // ReviewerStates contains the submitted review decisions per reviewer.
    // This is a lazy-loaded field; the zero value is LoadStatePending.
    // Populated by a secondary /reviews call (GitHub, Gitea) or /approvals (GitLab).
    ReviewerStates LoadResult[[]ReviewerState]

    // AggregateState is the computed roll-up used for list-view display and sorting.
    // Computed by the adapter after ReviewerStates is loaded; before that, it is
    // derived from RequestedReviewers alone (any requested reviewer → ReviewRequired).
    AggregateState AggregateReviewState
}

// CIStatus holds the overall CI result for the PR's head commit.
// Wrapped in LoadResult because availability and fetch cost vary by provider.
type CIStatus struct {
    State   CIState
    Summary string // human-readable summary, e.g., "3 checks passed, 1 failed"
}

// DiffStats holds size metrics that require the detail endpoint.
type DiffStats struct {
    Commits      int
    ChangedFiles int
    Additions    int
    Deletions    int
}
```

### The normalized PR type

```go
// PullRequest is the normalized internal representation of a pull request or
// merge request from any v1 provider. All display, filtering, sorting, and
// grouping operates on this type.
type PullRequest struct {
    // --- Identity ---

    // ProviderID is the provider-scoped internal ID (GitHub: int64, GitLab: int, Gitea: int64).
    // Stored as string to avoid type coupling. Used for deduplication and secondary fetches.
    ProviderID string

    // Number is the human-visible PR/MR number (e.g., #1234).
    Number int

    // Provider is the instance this PR came from.
    Provider ProviderInstance

    // Repo is the repository this PR targets.
    Repo Repository

    // URL is the canonical web URL for this PR.
    URL string

    // --- Content ---

    Title        string
    SourceBranch string // head branch (the branch being merged)
    TargetBranch string // base branch (the branch being merged into)
    HeadSHA      string // commit SHA at the tip of the source branch; used by the Event Engine to detect new commits
    Body         string // PR description; may be empty

    // --- State ---

    // State is the lifecycle state. Draft PRs are represented as PRStateDraft
    // rather than PRStateOpen to allow the display layer to filter or group them
    // without checking a separate boolean.
    State PRState

    // Draft is preserved as a separate field for providers that do not collapse
    // draft into state, and for filtering convenience.
    Draft bool

    // Mergeable is nullable because providers compute this asynchronously.
    // The zero value is MergeableStateUnknown.
    Mergeable MergeableState

    // --- Participants ---

    Author Author

    // --- Review ---

    Reviews ReviewSummary

    // --- CI ---

    // CI is lazy-loaded. For GitLab it may be populated from the list response.
    // For GitHub and Gitea/Forgejo it requires a secondary call.
    CI LoadResult[CIStatus]

    // --- Counts (eagerly loaded from list response) ---

    CommentCount int // general comments (not review comments)

    // --- Counts (lazy-loaded from detail endpoint) ---

    // Diff contains commit/file/line counts. Requires the PR detail endpoint.
    Diff LoadResult[DiffStats]

    // --- Labels ---

    Labels []Label

    // --- Timestamps ---

    CreatedAt time.Time
    UpdatedAt time.Time
    MergedAt  *time.Time // nil if not merged
}
```

### Design notes

**`PRState` vs. separate `Draft` bool:** The `State` enum includes `PRStateDraft` as a discrete state rather than encoding draft as `PRStateOpen + Draft: true`. This makes filter expressions simpler (compare one field, not two). The `Draft bool` field is retained alongside it for providers that need it separately and as a convenience for adapters that set `State` first and then apply the draft flag in a second pass.

**`ProviderID` as string:** Provider IDs are int64 in practice for all three v1 providers, but normalizing to string avoids coupling the schema to a numeric type and makes the field safe for any future provider (e.g., Bitbucket uses UUIDs).

**`LoadResult[T]` over `*T` pointers:** Raw pointer fields create ambiguity between "not loaded" and "explicitly absent." `LoadResult[T]` makes fetch lifecycle a first-class concern, enabling the TUI to show appropriate indicators (spinner for pending, dash for absent, value for loaded) without ad-hoc nil checks throughout the view layer. The generic form keeps the implementation compact and type-safe without requiring a separate wrapper type per field.

**No `Score` or `Urgency` field:** Priority is a view configuration concern, not a data property. A user's priority definition is a multi-key sort expression — `changed_files ASC, created_at DESC` — not a number. Sort expressions are transparent, serializable to config, and composable; a score collapses multiple signals into one opaque value that requires trusting someone else's weighting. The data model provides the raw fields; the query layer orders them.

**Consumer-agnostic design:** The normalized `PullRequest` type and provider adapters carry no presentation or transport concerns. The TUI is the first consumer but not the only valid one — an MCP server, HTTP handler, or one-shot CLI command all compose the same model and query layers without modification. Nothing in this type encodes how or where it will be rendered.

**`ReviewSummary` design:** Two levels of review data exist in the struct — `RequestedReviewers` (available at list time for GitHub and GitLab) and `ReviewerStates` (the loaded full set of submitted reviews). `AggregateState` is derived from whichever is more complete: before `ReviewerStates` loads, it is inferred from `RequestedReviewers`; after loading, it reflects the actual submitted decisions. This lets the list view show a useful triage signal immediately without waiting for secondary fetches.

**`Body` field:** The PR description is included for display in the detail pane. It may be empty for all three providers (optional field). Not used for filtering in v1.

---

## Consequences

### Provider adapter implementation

Each provider adapter implements a `Fetch` method (or equivalent within the Bubble Tea command pattern) that maps provider-specific API response types to `[]PullRequest`. Mapping rules:

- **GitHub:** `PullRequest.State` set from `state` + `draft` fields; `Reviews.RequestedReviewers` populated from `requested_reviewers[]`; `Reviews.ReviewerStates` starts as `LoadStatePending`; `CI` starts as `LoadStatePending` (fetched via GraphQL batch or `/check-runs`).
- **GitLab:** `PullRequest.State` set from `state` + `work_in_progress` (draft) fields; `Reviews.RequestedReviewers` populated from `reviewers[]`; `CI` populated immediately from `head_pipeline` (set to `LoadStateLoaded`).
- **Gitea/Forgejo:** `Reviews.RequestedReviewers` is empty at list time (not in list response); `CI` starts as `LoadStatePending` (Gitea Actions endpoint if available, otherwise `LoadStateAbsent`).

### Secondary fetch strategy

The Bubble Tea command model fires secondary fetch commands concurrently with list rendering. Each provider adapter exposes separate methods for enriching a `PullRequest` with review states and CI status. The enriched `PullRequest` value is sent back as a `tea.Msg` and merged into the in-memory model, triggering a re-render. The `LoadResult[T]` state transitions from `Pending → Loaded` (or `Pending → Error`) atomically per field.

### Filtering and sorting

Filter and sort expressions operate on `PullRequest` fields. The view layer treats `LoadStatePending` fields as "no signal" for sorting (sort-last) and shows a spinner/placeholder for filtering. `LoadStateAbsent` fields are treated as excluded from filter criteria rather than as failing the filter.

### Future extensibility

- **Additional providers** (Bitbucket, Azure DevOps): add a new `ProviderKind` constant and an adapter; the schema is not modified.
- **Issue tracker extension**: the `ProviderInstance` and `LoadResult[T]` patterns carry over to a normalized `Issue` type. The PR and Issue types share no embedding but follow the same structural pattern.
- **Write operations** (approve, comment): these do not change the data model; they add methods on the adapter interface that take `PullRequest.ProviderID` + `Provider` as inputs.
- **`AggregateReviewState` expansion**: if providers add new review states (e.g., GitLab adds a "requested changes" concept), the enum is extended and the aggregation logic updated; no change to the struct layout.
- **Additional consumers** (MCP server, HTTP API, CLI one-shot): the model and query layers are already consumer-agnostic; new transports add a thin assembly layer without touching this schema.
- **Event Engine (ADR-007)**: the `PullRequest` type is the unit of comparison in the delta engine. Identity is keyed on `(Provider.Kind, Provider.Host, ProviderID)`. Field-level diff relies on all `PullRequest` fields being comparable — no map fields are used; slices and structs are compared field-by-field. `HeadSHA` is the signal for `pr.new_commit` detection. `LoadResult[T]` state transitions (Pending → Loaded) do not trigger events; only value changes in loaded fields do.
