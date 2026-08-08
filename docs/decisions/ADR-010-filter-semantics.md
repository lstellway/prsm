# ADR-010: Filter Semantics

## Status

Accepted

## Context

ADR-006 specified the filter vocabulary. ADR-004 specified the PR data model. ADR-009 specified who owns lazy-load policy. Each is internally coherent. A review of `query/filter.go` at commit `560f656` found that the three interact in ways none of them anticipated, and that the interactions produce silently wrong results rather than errors.

Two concrete failures:

- **`state = "open"` hides every draft on GitHub.** ADR-004 encodes draft twice — `PRStateDraft` as a discrete state *and* a `Draft bool` — and `adapter/github/normalize.go:52` returns `PRStateDraft` for any draft PR, so a draft's `State` is never `PRStateOpen`. `matchesPRState` (`query/filter.go:148`) is bare equality. ADR-006 §1 (the `state` filter row) makes `"open"` the default `state`. The moment that default is implemented, prsm's baseline view drops every draft — on GitHub only. The same spec returns a different set on a provider using the `State=open, Draft=true` encoding, which cuts directly against "views are vendor-agnostic."
- **`review_status` matches nothing until reviews load.** `adapter/github/normalize.go:119` sets `AggregateState` to `review_required` when requested reviewers exist and leaves it `""` otherwise, so at list time it is only ever one of those two values. `matchesAggregateReviewState` (`query/filter.go:142-146`) is plain equality. `approved`, `changes_requested`, and `commented` therefore match nothing at all until `ReviewerStates` lazily loads — and ADR-009 §2 puts *when* that happens entirely in each consumer's hands. Those are ADR-006's triage queries #4 and #5. Meanwhile `ci_status = "failing"` has the opposite defect: it matches *everything* on first paint, because ADR-006's Option C passes pending through and every PR starts `Pending` (`adapter/github/normalize.go:33`). Two lazy fields, opposite treatments, nothing justifying the asymmetry.

Neither failure is a bug in one layer. Both are the seam between layers, and both are invisible: the user gets a plausible-looking list that is wrong.

The review surfaced four further ambiguities in the same seam — whether provider names compare case-sensitively, whether `provider` means `Provider.Name` or `Provider.Account`, which filter fields are universal, and where `ResolvedIdentities` belongs. All six are settled here. `docs/decisions/research/filter-semantics-open-questions.md` holds the full evidence, alternatives, and rejection reasoning; this ADR records the decisions.

Nothing in this ADR is in code yet.

## Decision

### 1. `state` semantics and draft

**`state = "open"` includes drafts, and `"draft"` is no longer a `filter.state` value.**

`matchesPRState(PRStateOpen)` also accepts `PRStateDraft`, per `model/pr.go:12`'s existing "open + draft" definition. Draft-ness is expressed *only* by the `draft` bool, so there is exactly one spelling for each query.

```
filter.state: open | closed | merged
filter.draft: true | false | unset

state = "open"                 → open + drafts
state = "open", draft = false  → open, no drafts
state = "open", draft = true   → drafts only
draft = true                   → all drafts, any state
state = "draft"                → config validation error
```

**Omitting `state` means all states.** ADR-006's `"open"` default is dropped. `State` stays a plain `string` and `""` keeps the "no filter" meaning it already has at `query/filter.go:64`, which is what ADR-006 §7 already describes as the "all PRs" baseline.

What follows:

- `matchesPRState` (`query/filter.go:148`) special-cases `PRStateOpen`. `parsePRState` (`query/filter.go:266`) and `validStates` (`config/load.go:172`) drop the `draft` arm.
- **No cross-field validation is needed.** `state = "draft"` + `draft = false` becomes unrepresentable rather than an unsatisfiable spec nothing rejects.
- `state = "open"` + `draft = true` now behaves identically on GitHub and on providers using the `State=open, Draft=true` encoding. The vendor-agnostic guarantee holds without an ADR-004 normalization mandate.
- **The model is unchanged.** `model.PRStateDraft` stays, adapters keep normalizing to it (`adapter/github/normalize.go:59`), and `matchesDraftState` keeps reading both encodings (`query/filter.go:157-162`). Only the filter vocabulary narrows.
- `config/load.go` needs no defaulting pass — the gap the review flagged closes by amending the ADR rather than by adding code. The shipped default config spells `state = "open"` explicitly in every view that wants it.
- Open+merged is not expressible. Accepted.
- No grouping or sort key reads `State`, so nothing else is affected.

Rejected: dropping `PRStateDraft` from the model as well — it would collapse `matchesDraftState` to a single bool compare, but it is an ADR-004 change and it removes the ability to represent a vendor that only ever reports "draft" as a state. `State []string` with OR semantics — the only option expressing open+merged, but it adds an empty-vs-unset ambiguity to a fourth field for a case nobody has asked for. A `state = "all"` sentinel — a value that is not a `PRState`, forcing `parsePRState` to return something other than the enum.

### 2. Lazy-field filter behavior and loading

**(a) `Compile` reports the lazy fields its predicate depends on; the consumer prefetches them.**

```go
func (filterSpec PRFilterSpec) Compile(
    identities model.ResolvedIdentities,
) (Predicate[model.PullRequest], []model.LazyField, error)
```

ADR-009 §2 is unchanged — lazy-load policy stays with each consumer. `Compile` reports a dependency; it does not schedule anything. This is the payoff for keeping the spec a declarative struct rather than an opaque closure, and it makes the struct form load-bearing rather than incidental. The TUI's cursor-driven policy gains one exception: batch-load the reported fields before first paint, then fall back to cursor-driven loading. A consumer that ignores the returned set gets today's degraded behavior — a documented consequence, not a silent one. `LazyField` enumerates `model.PullRequest`'s `LoadResult` fields (`ci`, `reviews`, `diff`), so it lives in `model` for the same reason as §6.

Rejected: assembly introspecting the spec (amends ADR-009 §2 and moves the spec-field→lazy-field coupling out of the layer that owns it); symmetric Option C passthrough for reviews (both filters would over-match on first paint, and it needs a UI affordance that does not exist); computing the full aggregate at list time (one extra REST call per PR per poll, which ADR-003's budget likely cannot absorb — worth revisiting if the GitHub adapter moves to GraphQL).

**(b) Assembly provides `EnsureLoaded`; each consumer decides when to call it.**

```go
// assembly (package prsm) — mechanism, per ADR-009 §2
func (client *Client) EnsureLoaded(ctx context.Context, fields []model.LazyField) error
```

Reporting the dependency is half the fix; something must act on it, and it must be the *same* something on every surface. ADR-009 §4 requires that a snapshot mean the same thing in-process and over the wire. If `prsm serve` completes its lazy fields and the TUI does not, `ci_status = "failing"` returns different sets depending on which surface you ask — the divergence ADR-009 calls a defect. So the mechanism is shared and the trigger is not, exactly the split ADR-009 §2 already draws: TUI on view activation, MCP per request, HTTP on a query parameter.

Constraints this puts on `EnsureLoaded`:

- **Write-back is required, not an optimization.** Results go *into* the snapshot. A per-request throwaway load costs one call per PR on every request and will exhaust the ADR-003 budget. With write-back the first call pays and the 60s poll refreshes via ETags for free.
- **Loads must be bounded and concurrent.** `LoadCI` and `LoadReviewerStates` are per-PR (`adapter/github/github.go:206`, `:246`). 50 PRs at ~200ms, 10 at a time, is roughly one second added to a first request.
- **A cap is needed, and hitting it must be reported.** A filter over 500 PRs is 500 calls. Silent truncation would read as "covered everything."
- **Enrichment can partially fail.** 3 of 50 come back `LoadStateError`; (c) and (d) cover that.

**(c) Pending matches, uniformly, for every lazy field.**

ADR-006's Option C generalizes from "CI only" to "any lazy field a filter depends on." `matchesAggregateReviewState` gains the same `IsPending()` guard `matchesCIState` already has at `query/filter.go:229`.

With `EnsureLoaded` in place this is a narrow case: a well-behaved consumer has loaded what it needs before applying the filter, so the pending window is only the sub-second gap for a PR that appeared between the load and the paint. In the TUI that is a brief extra row; the mirror choice would be a brief missing row. Symmetric, and low stakes. It is *not* symmetric for a consumer that skips `EnsureLoaded`: passthrough returns everything, which looks obviously broken and gets fixed; exclusion returns nothing, which reads as a confident "no PRs are failing" and never gets investigated. Same reasoning as `resolveMe` returning a non-match rather than comparing against `""`.

**(d) Unknown matches, known compares.**

Generalizes (c) to cover terminal load failure, which is permanent rather than transient. One rule across all four `LoadState` values:

```go
// unknown → match; known → compare
if pullRequest.CI.IsPending() || pullRequest.CI.IsError() {
    return true
}
actualState := model.CIStateNone // Absent: the provider has no CI to report
if ciStatus, ok := pullRequest.CI.Get(); ok {
    actualState = ciStatus.State
}
return actualState == wantedState
```

| `LoadState` | Meaning | Filter behavior |
|---|---|---|
| `Pending` | not fetched yet | match |
| `Error` | fetched and failed | match |
| `Absent` | provider has no such concept | compare as `none` |
| `Loaded` | known | compare |

What follows:

- Fixes the silent permanent lie at `query/filter.go:232`, where `Error` currently reads as `CIStateNone` — so a PR whose CI fetch failed both matched `ci_status = "none"` and was excluded from `ci_status = "failing"`, forever.
- A PR whose load failed stays visible rather than vanishing from every `ci_status` view at once. ADR-009 §3 requires that "no PRs" and "couldn't fetch" not look alike; a consumer can render the errored row distinctly because `LoadResult.Err()` is already there.
- Applies identically to `review_status` once `matchesAggregateReviewState` gets the same guard.
- Accepted cost: a PR that errors on every poll stays permanently visible in `ci_status` views it may not belong to. Visible-and-wrong beats invisible-and-wrong.

Rejected: excluding errors from all value filters (a failed fetch would silently remove the PR from every `ci_status` view simultaneously — the exact failure ADR-009 §3 names); an explicit `ci_status = "unknown"` value (most honest, but it adds vocabulary to TOML, the wire API, and `parseCIState`, needs a mirror in `review_status`, and it reopens (c) since `Pending` would stop passing through — revisit if users ask "what couldn't prsm determine?").

**Deferred:** whether `Absent` legitimately means `none`. Per CLAUDE.md, capability is a property of a *connection*, so "this provider has no CI concept" may warrant connection-level reporting rather than a per-PR value that silently satisfies `ci_status = "none"`. Deferred until a non-CI-bearing provider adapter exists — GitLab and Gitea are `Config`-only stubs, so nothing produces `Absent` today.

### 3. Provider instance names match exactly

**Provider names compare case-sensitively everywhere, and unknown names are rejected at config load.**

Provider names are user-chosen identifiers, so they compare exactly. Today they do not: `resolveMe` does an exact map lookup on `Provider.Name` (`query/filter.go:105`), `matchesAnyProviderName` does `strings.EqualFold` on the same field from the same config file (`query/filter.go:202`), and `config/load.go:72` rejects duplicate provider names case-sensitively — so a config may legally declare both `github` and `GitHub`, and `provider = "github"` matches both instances while `resolveMe` resolves only one.

- `query/filter.go:202` drops the `strings.EqualFold`.
- `query/filter.go:105` and `config/load.go:72` are unchanged; both are already exact.
- **New validation rule:** `filter.provider` must name a configured `[[providers]].name`. `seenProviderNames` is already built at `config/load.go:55`, so this is nearly free.

What follows: a typo (`GitHb`, or `GitHub` when the instance is `github`) fails loudly at config load instead of producing a silently empty view. That validation rule is what makes exact matching safe to demand. The STE-74 reasoning then holds uniformly — both sides of every provider-name comparison originate from the same configured instance, so a mismatch is an assembly bug worth surfacing rather than papering over.

Rejected: folding everywhere (it would also require folding `config/load.go:72`, or `github` and `GitHub` stay legally distinct instances while one filter matches both); normalizing names at load into a separate lookup key (one rule enforced at the boundary, but it is a `ProviderInstance` model change for a problem the validation rule already solves).

### 4. `provider` grouping keys on `Provider.Name`

**`provider` means `Provider.Name` in both filter and group.**

The filter keys on `Provider.Name` — the config alias, e.g. `github-personal` (`query/filter.go:202`) — while grouping keys on `Provider.Account`, the resolved login, e.g. `acme` (`query/group.go:85`). ADR-006 §1 and §5 say the same ambiguous thing in both tables, so the ADR cannot arbitrate. `query/group.go:85` changes to return `pullRequest.Provider.Name`. One line.

What follows:

- One meaning for "provider" across the query layer, matching `model/provider.go:15`'s existing documentation that `Name` "matches provider filter in views."
- `Name` is always populated; `Account` is set only after `ResolveIdentity` succeeds (`adapter/github/github.go:89`), so grouping on it collapsed every PR from an identity-failed provider under the key `""`. That bug disappears.
- Group headers read as the config alias (`github-personal`) rather than the org (`acme`) — what the user wrote and can search for.

Rejected: keying on `Account` and changing the filter to match. Headers would read better, but the filter vocabulary would then depend on a value that exists only after a successful identity call, so a filter could silently stop matching when a token expires, and two instances pointed at one org become indistinguishable. Rendering the account alongside the name in the header stays available to the TUI — that is a presentation concern, not a query-layer one.

### 5. Universal filter fields

**Universal is `Author`, `Repo`, `Provider`, `Label`, `StalenessGTE`, `TargetBranch`. `State` is per-resource.**

Three definitions of "universal" are in play and none match: ADR-000 Layer 3 names six fields, `query/filter.go:18-22` holds three of those six, and `config/config.go:70-76` groups seven under a literal `// Universal filter fields` comment. Whoever writes `IssueFilterSpec` has a coin flip.

```go
type BaseFilterSpec struct {
    Author       string
    Repo         []string
    Provider     []string
    Label        []string
    StalenessGTE int
    TargetBranch string
}

type PRFilterSpec struct {
    BaseFilterSpec
    State        string
    Draft        *bool
    Reviewer     string
    ReviewStatus string
    CIStatus     string
}
```

What follows:

- `Label`, `StalenessGTE`, and `TargetBranch` move from `PRFilterSpec` into `BaseFilterSpec`.
- **`State` stays per-resource**, and §1 makes that principled rather than arbitrary: the field name is shared but the value domain is not — PRs are `open|closed|merged`, issues would be `open|closed`. ADR-000 Layer 3 listing `State` as universal is the error.
- `config/load.go:171` moves `state` validation into a per-resource switch, and `config/load.go:195`'s resource-incompatibility block gains `state` as a value-domain check rather than a presence check.

Rejected: flattening `BaseFilterSpec` away until a second resource type exists (the embed provides zero code reuse today and costs every construction site `PRFilterSpec{BaseFilterSpec: BaseFilterSpec{...}}`, but reversing an ADR-000 commitment and re-adding it later is churn).

**Deferred:** the generic pass itself. The split shares *fields* but not *behavior* — every predicate constructor is hard-typed `Predicate[model.PullRequest]`, and Go methods cannot take type parameters, so a second resource type copies five constructors verbatim. Genuine sharing needs free generic functions over an accessor interface or extractor funcs. Designing that interface with no second resource type to validate it against is premature; record on `BaseFilterSpec` that its predicates are PR-typed and that a second resource type triggers the work.

### 6. `ResolvedIdentities` lives in `model` and is cloned on compile

**Move the type to `model`, hold `Identity`, and clone it in `Compile`.**

```go
// model
type ResolvedIdentities map[string]Identity // keyed by ProviderInstance.Name

// query
func (filterSpec PRFilterSpec) Compile(
    identities model.ResolvedIdentities,
) (Predicate[model.PullRequest], []model.LazyField, error) {
    identities = maps.Clone(identities)
    ...
}
```

The type is declared in `query` today (`query/filter.go:15`). That is legal — ADR-000's layer boundary rules let assembly import query — but it blocks a planned consumer: ADR-000 Layer 4 has hook filter expressions reusing the `FilterSpec` vocabulary, the same rules constrain the Event Engine to "the resource model only," and `config.HookConfig` already carries a `FilterConfig` (`config/config.go:104`). So `event` will need to resolve `"me"` and will not be permitted to name `query.ResolvedIdentities`. The move costs one line.

What follows:

- The value type becomes `model.Identity`. `model.Author` is an alias for `Identity`, so this is a zero-behavior rename, and it names the type honestly — the map resolves reviewers as much as authors (`query/filter.go:129`).
- `maps.Clone` makes the compiled predicate immutable, so ADR-006's "recompile when `resolvedMe` becomes available" (§ Bubble Tea integration) is enforced rather than merely documented. Without it, assembly writes into the map on identity recovery while the TUI reads it per keystroke — the same shape as the data race fixed in `6b7512d`.
- Pairs with `LazyField` from §2: `model` owns the vocabulary that crosses layers.
- The parameter is renamed off `resolvedMe` at the same time. It names a map as though it were a single value, and `resolveMe(raw, pullRequest, resolvedMe)` differs from its own function name by one character.

### 7. `AggregateReviewState` distinguishes computed from uncomputed

**`AggregateReviewStateNone` is `"none"`. The zero value `""` means "not yet computed" and is not an enum member.**

```go
AggregateReviewStateNone AggregateReviewState = "none" // computed: no reviews
// "" is the zero value and means "not computed". It is not a member of this enum.
```

`model/review.go:19` currently sets `None` to `""` — the zero value — so "computed, and there are no reviews" is indistinguishable from "never computed". ADR-004 §Review state vocabulary already specifies `"none"`; the implementation regressed from its own ADR. That collision is why `review_status = "none"` matches a PR approved by three reviewers whose aggregate was never populated.

Separating them makes the field self-describing, which in turn removes a cross-field dependency §2 would otherwise have needed:

```go
if pullRequest.Reviews.AggregateState == "" {
    return true // not computed → unknown → match
}
return pullRequest.Reviews.AggregateState == wantedState
```

What follows:

- `adapter/github/normalize.go:119` keeps leaving the field at `""` when no reviewers are requested. That is now correct and meaningful rather than an oversight — it claims nothing until reviews load.
- **A derived `review_required` is compared, not passed through.** ADR-006 §2 calls the `RequestedReviewers`-derived value a reliable lower bound; a blanket "still loading → match" would have discarded it, so a PR known to need review would have matched `review_status = "approved"`. It now falls into the compare branch.
- §2's rule for `review_status` reads one field, not two. No `IsPending()` check against `ReviewerStates`.
- `TestCompile_ReviewStatus_None` pins the old behavior and must be updated.

**The asymmetry with `ci_status` is deliberate, and it follows a rule.** `CI` is a `LoadResult`, so its unknown state is `IsPending() || IsError()`; `AggregateState` is a bare enum, so its unknown state is `""`. Both implement §2's "unknown matches, known compares" rule and differ only in how each field spells "unknown".

The general convention is recorded in **ADR-004 §Design notes → Unknown values**, in two axes:

1. **Within an enum**, the zero value means *unknown* and gets an explicit name; every real answer, including "genuinely none", gets a non-empty value. The sentinel stays at the zero value so the invariant holds without a constructor.
2. **`LoadResult[T]` wraps a field iff it is fetched** over the network on its own schedule. A locally computed field is never wrapped.

The rule is derived from the model rather than imposed on it: of `model`'s six string enums, only two put a named member at the zero value, and only one of those — `AggregateReviewStateNone` — puts a *real answer* there. `MergeableStateUnknown = ""` is the correct shape and the reference example. The same collision has now been found four times (`query/filter.go:232`, `model/review.go:19`, `query/group.go:88`, and ADR-004's self-contradiction about `MergeableState`), which is why it is written down instead of left to precedent.

Naming the sentinel is part of this decision: `AggregateReviewStateUnknown = ""` declared alongside the other members, with an `IsKnown()` predicate so the check has one spelling rather than a bare `== ""` at each of the three call sites. Note that the compiler cannot catch two constants in a typed group sharing a value — which is how the original defect survived — so a test asserting the members are pairwise distinct belongs with the fix.

Rejected: `AggregateState LoadResult[AggregateReviewState]` — it expresses "not computed" in the type rather than as a zero value and would let `review_status` reuse `ci_status`'s rule verbatim, but it is a `ReviewSummary` model change touching every read site, and the derived-vs-fully-computed distinction would still need a home. Revisit if a second field develops the same shape.

### 8. Grouping on a lazy field

**Unresolved rows get their own group, and group keys report lazy-field dependencies the way filters do.**

§2 is a filter rule, and "unknown matches" has no grouping analogue: every row must land in exactly one bucket. `review_status` is a group key (ADR-006 §5), and `query/group.go:88-91` maps an empty `AggregateState` to the `"none"` group — so PRs whose reviews have not loaded pile in with genuinely-unreviewed ones. That is the §7 conflation, one layer up.

Two parts:

1. **`GroupSpec` reports its lazy-field dependency**, the same mechanism §2a gives `Compile`. A consumer that calls `EnsureLoaded` has the data before it renders, so the bucket is rare rather than routine. `GroupSpec.ValidateForResource` (`query/group.go:27`) establishes that `GroupSpec` is a reasonable home for such a method.
2. **Anything still unresolved gets a distinct group**, rendered last, never folded into a real one.

```
▾ changes requested (2)
▾ review required (5)
▾ none (1)
▾ not loaded (3)     ← distinct, last, usually empty
```

What follows:

- The bucket needs a key that cannot collide with a real `AggregateReviewState`. §7 takes `"none"` and reserves `""` as the sentinel, so the group needs its own key and display label.
- Consistent with §2's principle that unknown is visible rather than disguised as a real answer. A PR with three approvals must not sit under a header reading `none`.
- `review_status` is the only lazy group key today (`query/group.go:13-17`), but the mechanism should be general.

Rejected: the bucket without the reporting mechanism (still honest, but a grouped view would show one large "not loaded" pile on first paint — accurate and useless); suppressing unresolved rows until they resolve (rows appearing and vanishing under a grouped header is worse than a visible bucket).

## Consequences

**Easier:**

- Every filter value now has exactly one spelling and one meaning, and the same spec returns the same set on every provider and every consumer surface.
- The two lazy fields get one rule instead of two opposite ones, and that rule extends to `Diff` and to any lazy field a future resource type adds.
- Wrong filter values fail at config load rather than producing an empty view — §1's `state = "draft"` and §3's unknown provider name.
- A PR whose enrichment failed stays visible and can be rendered as errored, satisfying ADR-009 §3 at the query layer rather than only at the adapter.
- `model` owning `ResolvedIdentities` and `LazyField` unblocks the Event Engine without an ADR-000 boundary exception.

**Harder — accepted costs:**

- **`open` and `merged` together is not expressible.** Dropping the `state` default removes the need for an "all" sentinel, but AND-only composition over a single-valued `state` leaves no spelling for a two-value union.
- **A PR that errors on every poll stays permanently visible** in `ci_status` and `review_status` views it may not belong to.
- **`Compile` grows a second return value**, so every call site changes, and a consumer that ignores it silently gets the degraded first-paint behavior this ADR exists to fix.
- **`EnsureLoaded` puts a first-request latency and rate-limit cost where there was none** — bounded and reported, but real, and it is now assembly's job to enforce the cap and surface when it is hit.
- **The construction cost of `BaseFilterSpec` is paid before any reuse is realized.** Every construction site writes `PRFilterSpec{BaseFilterSpec: BaseFilterSpec{...}}` for an embed that shares no behavior until the generic pass lands.

## Amendments to prior ADRs

- **ADR-000 Layer 3** — the universal field list is wrong. `State` is removed from it and `Label`, `TargetBranch` are added; the set is `Author`, `Repo`, `Provider`, `Label`, `StalenessGTE`, `TargetBranch` (§5).
- **ADR-004 §PR state enums / §Design notes** — the `PRStateDraft` rationale is withdrawn. "Draft is surfaced separately for triage" and "makes filter expressions simpler (compare one field, not two)" no longer hold: `filter.state` has no `draft` value and draft-ness is expressed only by `draft`. The type and both fields are unchanged (§1).
- **ADR-006 §1** — the `state` row loses "— default `"open"`" and drops `draft` from its value list; the sample config in §3 keeps `state = "open"` but loses the `# default; can omit` comment (§1).
- **ADR-006 §1** — the `provider` row is corrected from `Provider.Account` to `Provider.Name` (§4).
- **ADR-006 §1** — the `target_branch` row loses its `release/*` example. The implementation is a literal `strings.Contains`; either the example goes or the implementation owes a glob.
- **ADR-006 §2** — Option C generalizes from "CI only" to any lazy field a filter depends on, and extends to terminal load failure under the `unknown matches, known compares` rule. `LoadStateError` is no longer treated as `"none"` (§2c, §2d).
- **ADR-006 §5** — the `provider` grouping row is corrected from `Provider.Account` to `Provider.Name` (§4).
- **ADR-006 §Go predicate type** — records that the declarative struct spec is load-bearing, not stylistic: it is what lets `Compile` report its lazy-field dependencies (§2a).
- **ADR-006 §Startup: resolving `"me"` sentinels** — its "filters using `me` for that provider are **disabled** with a startup warning" reads as pass-through; the behavior is match-nothing per ADR-009 §3, which is what the code implements. The same section still describes identities as `map[ProviderKind]ResolvedIdentity`; STE-74 added the superseded note under ADR-006 §Go predicate type but left this sentence.
- **ADR-005 §Validation and error reporting at startup** — a ninth rule: `filter.provider` values must name a configured `[[providers]].name` (§3). Rule 4's enum vocabulary for `filter.state` narrows to `open|closed|merged` (§1).
- **ADR-005 §View definition schema** — the universal field table held a fourth, different definition of the universal set. `state` moves to the PR-specific table and `target_branch` is added to the universal one (§5); `state` loses its default (§1).
- **ADR-005 §Complete annotated example config** — the `state` comment and the `my-reviews` view, which omitted `state` and so would now span closed and merged (§1).
- **ADR-000 §Named view definitions** — the `my-reviews` example gains `state = "open"` for the same reason (§1). Dropping the default changed the meaning of every example that relied on it; the shipped config at `config/defaults.go` needs the same fix in code.
- **ADR-004 §Review state vocabulary** — no change to the ADR; `model/review.go:19` regressed from it by setting `AggregateReviewStateNone` to `""` instead of `"none"`. The ADR is restored as the source of truth, and `""` is documented as the non-member zero value meaning "not computed" (§7).
- **ADR-004 §PR state enums** — the `MergeableState` block assigns `MergeableStateUnknown = "unknown"` while §The normalized PR type says the zero value *is* `MergeableStateUnknown`. The ADR contradicted itself; `""` is correct and the block is the error (§7).
- **ADR-004 §Design notes** — gains "Unknown values", the two-axis rule for how a model field spells unknown: the enum's zero value is the named unknown sentinel, and `LoadResult[T]` wraps a field iff it is fetched rather than computed (§7).
- **ADR-006 §5** — grouping gains a distinct bucket for rows whose lazy group-key field has not resolved, and `GroupSpec` reports lazy-field dependencies as `Compile` does (§8).
- **ADR-009 §2** — `EnsureLoaded` is added to the assembly layer's owned responsibilities, with write-back into the snapshot, bounded concurrency, and a reported cap. **The policy split is unchanged** — *when* to trigger lazy loads remains each consumer's decision. The addition is the shared mechanism, which is what keeps the library and wire surfaces semantically consistent per ADR-009 §4 (§2b).

## References

- ADR-000: System Architecture — layer boundaries, the universal field list amended here
- ADR-003: Liveness Model — the rate-limit budget `EnsureLoaded` spends against
- ADR-004: PR Data Model — the dual draft encoding and `LoadResult[T]` lifecycle
- ADR-006: Filtering and Grouping — the filter vocabulary this ADR corrects
- ADR-009: Assembly Layer and Library Surface — the consumer-owned lazy-load policy and the surface-consistency requirement
- `docs/decisions/research/filter-semantics-open-questions.md` — full evidence, alternatives, and rejection reasoning for all six decisions
