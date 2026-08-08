# Filter semantics — open questions

Working doc. Records design questions surfaced by a review of `query/filter.go`, the
evidence for each, and the decision once made. Decisions here graduate into amendments
to ADR-004 / ADR-006 / ADR-009 and then into code.

Status: **all nine questions decided; the ADR amendments are written.** Nothing here is
in code yet.

Q1–Q6 came from the review of `query/filter.go`. Q7–Q9 surfaced while amending the ADRs
and are recorded at the bottom.

The decisions are recorded in **ADR-010: Filter Semantics**; this doc keeps the evidence,
the alternatives, and the rejection reasoning behind them.

Every claim below was verified against the source at commit `560f656`.

## ADR amendments these decisions require

All are written. ADR-005 was not in the original list — it turned out to hold a fourth,
different definition of the universal filter set, and it is the user-facing config
reference, so it mattered most.

| ADR | Change |
|---|---|
| ADR-005 | §Validation: a ninth rule — `filter.provider` must name a configured provider (Q3) |
| ADR-005 | §View definition schema: `state` moves out of the universal field table, `target_branch` moves in (Q1, Q5) |
| ADR-005 | The annotated sample config and the `my-reviews` example view (Q1) |
| ADR-000 | The `my-reviews` example gains `state = "open"` — without it the view now spans closed and merged (Q1) |
| ADR-004 | `model/pr.go:12`'s "surfaced separately for triage filtering" rationale for `PRStateDraft` no longer holds (Q1) |
| ADR-006 | Drop the `state` default in §1 and the `# default` comment in §3's sample config; drop `draft` from the `state` value list (Q1) |
| ADR-006 | Generalize Option C from "CI only" to "any lazy field", and extend it to terminal load failure (Q2c, Q2d) |
| ADR-006 | Record that the declarative struct spec is load-bearing — it's what lets `Compile` report lazy-field dependencies (Q2) |
| ADR-006 | Correct §1's `provider` filter row and §5's `provider` grouping row, which describe `provider` as `Provider.Account` (Q4) |
| ADR-006 | Fix the stale text in §Startup: resolving `"me"` sentinels (see the bottom of this doc) |
| ADR-006 | `target_branch`'s `release/*` example in §1 doesn't work — it's a literal `strings.Contains` |
| ADR-009 | Add `EnsureLoaded` as an assembly-owned mechanism, with write-back, bounded concurrency, and a reported cap (Q2b). §2's policy split is unchanged |
| ADR-000 | Layer 3's universal field list is wrong — `State` is per-resource (Q5) |

---

## Q1 — What does `state` mean?

Three tangled sub-questions. They have to be answered together.

**Evidence**

- `adapter/github/normalize.go:52` — `normalizePRState` returns `PRStateDraft` for any
  draft PR, so a draft's `State` is *never* `PRStateOpen` on GitHub.
- `model/pr.go:12` — documents `PRStateDraft` as "open + draft".
- `query/filter.go:148` — `matchesPRState` is bare equality.
- `query/filter.go:157-162` — `matchesDraftState` correctly handles *both* encodings
  (`State == PRStateDraft || Draft`).
- ADR-006 §1 (the `state` filter row) — `state` is specified with "default `"open"`".
- ADR-006 §3 — the sample config comments `state = "open"` as "# default; can omit".
- `config/load.go:171` — validates the value; never applies the default.
- `query/filter.go:64` — `""` already means "no filter".

**Consequences as written**

- `state = "open"` silently excludes every draft on GitHub. The moment ADR-006's
  default is implemented, the baseline view hides all drafts.
- `state = "draft"` + `draft = false` is unsatisfiable by construction — nothing
  rejects it, the view is just empty.
- `state = "open"` + `draft = true` works on a provider using the `State=open,
  Draft=true` encoding and matches nothing on GitHub. Same spec, different result per
  vendor — cuts against "views are vendor-agnostic".
- Because `""` means "no filter", once it defaults to `"open"` there is no spelling for
  "open and merged".

**Sub-questions**

1. Does `state = "open"` include drafts?
2. Is the ADR-006 default actually implemented, and where — config load, or `Compile`?
3. How does a user ask for all states?

**Decision (1) — `state = "open"` includes drafts, and `"draft"` is no longer a
`filter.state` value.**

`matchesPRState(PRStateOpen)` also accepts `PRStateDraft`, per `model/pr.go:12`'s
existing "open + draft" definition. Draft-ness is expressed *only* by the `draft`
bool, so there is exactly one spelling for each query.

```
filter.state: open | closed | merged
filter.draft: true | false | unset

state = "open"                 → open + drafts
state = "open", draft = false  → open, no drafts
state = "open", draft = true   → drafts only
draft = true                   → all drafts, any state
state = "draft"                → config validation error
```

Follows from this:

- `matchesPRState` (`query/filter.go:148`) special-cases `PRStateOpen`.
- `parsePRState` (`query/filter.go:266`) and `config/load.go:172`'s `validStates` drop
  the `draft` arm.
- **No cross-field validation is needed** — `state = "draft"` + `draft = false` becomes
  unrepresentable rather than an error case to detect.
- `state = "open"` + `draft = true` now behaves identically on GitHub and on providers
  using the `State=open, Draft=true` encoding — the vendor-agnostic guarantee holds
  without an ADR-004 normalization mandate.
- **The model is unchanged.** `model.PRStateDraft` stays; adapters keep normalizing to
  it (`adapter/github/normalize.go:59`) and `matchesDraftState` keeps reading both
  encodings (`query/filter.go:159`). Only the filter vocabulary narrows.
- `model/pr.go:12`'s "surfaced separately for triage filtering" comment needs
  rewording — that rationale no longer holds.
- No grouping or sort key reads `State`, so nothing else is affected.

Rejected: dropping `PRStateDraft` from the model too — it would collapse
`matchesDraftState` to a single bool compare, but it's an ADR-004 change and it removes
the ability to represent a vendor that only ever reports "draft" as a state.

**Decision (2), (3) — drop ADR-006's `state` default; omitted means all states.**

`State` stays a plain `string`. `""` keeps its existing meaning of "no filter", which
is already what ADR-006 §7 describes as the "all PRs" baseline. Open+merged is not
expressible; that's accepted.

Follows from this:

- ADR-006 §1's `state` row loses "— default `"open"`".
- ADR-006 §3's sample keeps `state = "open"` but drops the `# default; can omit`
  comment.
- `config/load.go` needs no defaulting pass — the gap flagged in the review closes by
  amending the ADR rather than by adding code.
- The shipped default config should spell `state = "open"` explicitly in every view
  that wants it.

Rejected: `State []string` with OR semantics (the only option expressing open+merged,
but adds an empty-vs-unset ambiguity to a fourth field for a case nobody has asked
for), and a `state = "all"` sentinel (a value that isn't a `PRState`, forcing
`parsePRState` to return something other than the enum).

---

## Q2 — Who triggers lazy loads so filters are correct?

The question that changes the most code.

**Evidence**

- `adapter/github/normalize.go:119` — `AggregateState` is set to `review_required`
  when requested reviewers exist, and left `""` otherwise. At list time the field is
  only ever `""` or `review_required`.
- `query/filter.go:142-146` — `matchesAggregateReviewState` is plain equality, no
  passthrough.
- `adapter/github/normalize.go:33` — every PR's `CI` starts `Pending`.
- `query/filter.go:227-238` — `matchesCIState` returns `true` for pending CI (ADR-006
  Option C).
- ADR-006 §2 — Option C assumes "once the CI data loads, the row is immediately
  re-evaluated".
- ADR-009 §2 — *when* to trigger lazy loads is each consumer's policy; the TUI's is
  "on cursor movement".

**Consequences as written**

- `review_status` = `approved` / `changes_requested` / `commented` match **nothing**
  until `ReviewerStates` lazily loads. Those are ADR-006's triage queries #4 and #5.
- `ci_status = "failing"` matches **everything** on first paint, decaying toward
  correctness only along the path the cursor happens to walk.
- The two lazy fields get opposite treatments — CI optimistic, reviews pessimistic —
  with nothing justifying the asymmetry and no way for a user to predict it.

**Decision — `Compile` reports the lazy fields its predicate depends on; the consumer
prefetches them.**

```go
func (filterSpec PRFilterSpec) Compile(
    identities ResolvedIdentities,
) (Predicate[model.PullRequest], []LazyField, error)
```

Follows from this:

- ADR-009 §2 is **unchanged** — lazy-load policy stays with each consumer. `Compile`
  reports a dependency; it does not schedule anything.
- This is the payoff for keeping the spec a declarative struct rather than an opaque
  closure. Worth stating in ADR-006 as a reason the struct form is load-bearing.
- The TUI's cursor-driven policy gains one exception: batch-load the reported fields
  before first paint, then fall back to cursor-driven loading.
- A consumer that ignores the returned set gets today's degraded behavior. That is a
  documented consequence, not a silent one.
- `LazyField` needs a home. It enumerates `model.PullRequest`'s `LoadResult` fields
  (`ci`, `reviews`, `diff`), so `model` is the natural place — same reasoning as Q6.

Rejected: assembly introspecting the spec (amends ADR-009 §2 and moves the
spec-field→lazy-field coupling out of the layer that owns it); symmetric Option C
passthrough for reviews (both filters would over-match on first paint, and it needs a
UI affordance that doesn't exist); computing the full aggregate at list time (one
extra REST call per PR per poll, which ADR-003's budget likely can't absorb — worth
revisiting if the GitHub adapter moves to GraphQL).

**Decision (b) — assembly provides `EnsureLoaded`; each consumer decides when to call
it.**

Reporting the dependency is only half the fix — something has to act on it, and it must
be the *same* something on every surface. ADR-009 §4 requires that a snapshot mean the
same thing in-process and over the wire; if `prsm serve` completes its lazy fields and
the TUI doesn't, `ci_status = "failing"` returns different sets depending on which
surface you ask. That is the divergence ADR-009 calls a defect.

So the mechanism is shared and the trigger is not — exactly the split ADR-009 §2
already draws:

```go
// assembly (package prsm) — mechanism, per ADR-009 §2
func (client *Client) EnsureLoaded(ctx context.Context, fields []model.LazyField) error
```

Sequence, one-shot consumer, filter `ci_status = "failing"`:

```
MCP client       MCP server        prsm.Client       GitHub adapter    GitHub API
    │ list_prs       │                   │                 │               │
    ├───────────────>│                   │                 │               │
    │                │  Snapshot()       │                 │               │
    │                ├──────────────────>│                 │               │
    │                │<─ ─ ─ ─ ─ ─ ─ ─ ─ ┤  50 PRs, CI Pending             │
    │                │                   │                 │               │
    │                │ Compile() ──> predicate, needs["ci"] │               │
    │                │                   │                 │               │
    │                │ EnsureLoaded(["ci"])                 │               │
    │                ├──────────────────>│  LoadCI(pr) ×50 │               │
    │                │                   ├────────────────>├──────────────>│
    │                │                   │<─ ─ ─ ─ ─ ─ ─ ─ ┤<─ ─ ─ ─ ─ ─ ─ ┤
    │                │<─ ─ ─ ─ ─ ─ ─ ─ ─ ┤  written back into the snapshot │
    │                │ Apply()           │                 │               │
    │<───────────────┤                   │                 │               │
    │ 3 PRs, correct │                   │                 │               │
```

Trigger policy per consumer, unchanged from ADR-009 §2 — TUI on view activation, MCP
per request, HTTP on a query parameter.

Constraints this puts on `EnsureLoaded`:

- **Write-back is required, not an optimization.** Results go *into* the snapshot. A
  per-request throwaway load costs one call per PR on every request and will exhaust
  the ADR-003 budget. With write-back, the first call pays and the 60s poll refreshes
  via ETags for free.
- **Loads must be bounded and concurrent.** `LoadCI` and `LoadReviewerStates` are
  per-PR (`adapter/github/github.go:206`, `:246`). 50 PRs at ~200ms, 10 at a time, is
  roughly one second added to a first request.
- **A cap is needed, and hitting it must be reported.** A filter over 500 PRs is 500
  calls. Silent truncation would read as "covered everything".
- **Enrichment can partially fail** — 3 of 50 come back `LoadStateError`. That is Q2c,
  and it is not solved by this decision.

**Decision (c) — pending matches, uniformly, for every lazy field.**

ADR-006's Option C generalizes from "CI only" to "any lazy field a filter depends on".
`matchesAggregateReviewState` gains the same `IsPending()` guard `matchesCIState`
already has at `query/filter.go:229`.

With `EnsureLoaded` in place this is a narrow case: every well-behaved consumer has
loaded what it needs before applying the filter, so the pending window is only the
sub-second gap for a PR that appeared between the load and the paint. In the TUI that
is a brief extra row; the mirror choice would be a brief missing row. Symmetric, and
low stakes.

It is not symmetric for a consumer that skips `EnsureLoaded`. Passthrough returns
everything, which looks obviously broken and gets fixed. Exclusion returns nothing,
which reads as a confident "no PRs are failing" and never gets investigated. Same
reasoning as `resolveMe` returning a non-match rather than comparing against `""`.

**Decision (d) — unknown matches, known compares.**

Generalizes (c) to cover terminal load failure, which is permanent rather than
transient. One rule across all four `LoadState` values:

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

Follows from this:

- Fixes the silent permanent lie at `query/filter.go:232`, where `Error` currently
  reads as `CIStateNone` — so a PR whose CI fetch failed both matched
  `ci_status = "none"` and was excluded from `ci_status = "failing"`, forever.
- A PR whose load failed stays visible rather than vanishing from every `ci_status`
  view at once. ADR-009 §3 requires that "no PRs" and "couldn't fetch" not look alike;
  a consumer can render the errored row distinctly because `LoadResult.Err()` is
  already there.
- Applies identically to `review_status` once `matchesAggregateReviewState` gets the
  same guard.
- Accepted cost: a PR that errors on every poll stays permanently visible in
  `ci_status` views it may not belong to. Visible-and-wrong beats invisible-and-wrong.

Rejected: excluding errors from all value filters (a failed fetch would silently remove
the PR from every `ci_status` view simultaneously — the exact failure ADR-009 §3 names);
an explicit `ci_status = "unknown"` value (most honest, but it adds vocabulary to TOML,
the wire API and `parseCIState`, needs a mirror in `review_status`, and it reopens (c)
since `Pending` would stop passing through — revisit if users ask for "what couldn't
prsm determine?").

**Still open, deliberately:** whether `Absent` legitimately means `none`. Per CLAUDE.md,
capability is a property of a *connection*, so "this provider has no CI concept" may
warrant connection-level reporting rather than a per-PR value that silently satisfies
`ci_status = "none"`. Deferred until a non-CI-bearing provider adapter exists — GitLab
and Gitea are `Config`-only stubs today, so nothing produces `Absent` yet.

---

## Q3 — Are provider instance names case-sensitive?

One answer settles three sites.

**Evidence**

- `query/filter.go:105` — `resolveMe` does an **exact** map lookup on
  `Provider.Name`. The STE-74 commit defends this: both sides originate from the same
  configured instance, so a mismatch is an assembly bug worth surfacing.
- `query/filter.go:202` — `matchesAnyProviderName` does `strings.EqualFold` on the
  same field, sourced from the same config file.
- `config/load.go:72` — rejects duplicate provider names **case-sensitively**, so a
  config may legally declare both `github` and `GitHub` as distinct instances.

**Consequence as written:** `provider = "github"` matches both instances while
`resolveMe` resolves only one.

**Decision — case-sensitive everywhere, and reject unknown names at config load.**

Provider names are user-chosen identifiers, so they compare exactly.

- `query/filter.go:202` drops the `strings.EqualFold`.
- `query/filter.go:105` and `config/load.go:72` are unchanged — both already exact.
- **New validation rule:** `filter.provider` must name a configured
  `[[providers]].name`. `seenProviderNames` is already built at `config/load.go:55`, so
  this is nearly free.

Follows from this:

- A typo (`GitHb`, or `GitHub` when the instance is `github`) fails loudly at config
  load instead of producing a silently empty view. This closes one of the validation
  gaps on the deferred list and is what makes exact matching safe to demand.
- The STE-74 reasoning holds uniformly: both sides of every provider-name comparison
  originate from the same configured instance, so a mismatch is a bug worth surfacing
  rather than papering over.

Rejected: folding everywhere (would also require folding `config/load.go:72`, or
`github` and `GitHub` stay legally distinct instances while one filter matches both);
normalizing names at load into a separate lookup key (one rule enforced at the
boundary, but it's a `ProviderInstance` model change for a problem the validation rule
already solves).

---

## Q4 — Does `provider` mean `Name` or `Account`?

**Evidence**

- `query/filter.go:202` — filters on `Provider.Name` (the config alias, e.g.
  `github-personal`).
- `query/group.go:85` — groups on `Provider.Account` (the resolved login, e.g. `acme`).
- `model/provider.go:15` — documents `Name` as "matches provider filter in views".
- `adapter/github/github.go:89` — populates `Account` only after `ResolveIdentity`
  succeeds.
- ADR-006 §1 and §5 say the same ambiguous thing in both tables, so the ADR can't
  arbitrate.

**Consequence as written:** filter on `github-personal`, group header renders `acme`.
On a provider whose identity call failed, every group collapses under `""`.

**Decision — `provider` means `Provider.Name` in both filter and group.**

`query/group.go:85` changes to return `pullRequest.Provider.Name`. One-line change.

Follows from this:

- One meaning for "provider" across the query layer, matching `model/provider.go:15`'s
  existing documentation.
- `Name` is always populated; `Account` is set only after `ResolveIdentity` succeeds
  (`adapter/github/github.go:89`), so grouping on it collapsed every PR from an
  identity-failed provider under the key `""`. That bug disappears.
- Group headers read as the config alias (`github-personal`) rather than the org
  (`acme`) — which is what the user wrote and can search for.
- ADR-006 §1 and §5 both say the same ambiguous thing ("`Provider.Account` name from
  config… matches the provider's configured `name` field") and need correcting.

Rejected: keying on `Account` and changing the filter to match (headers would read
better, but the filter vocabulary would depend on a value that only exists after a
successful identity call, so a filter could silently stop matching when a token
expires, and two instances pointed at one org become indistinguishable). Rendering the
account alongside the name in the header remains available to the TUI later — it's a
presentation concern, not a query-layer one.

---

## Q5 — Which filter fields are universal?

**Evidence**

- ADR-000 Layer 3 — "`BaseFilterSpec` holds universal fields (`Author`, `Label`, `Repo`,
  `Provider`, `StalenessGTE`, `State`)."
- `query/filter.go:18-22` — holds three of those six.
- `config/config.go:70-76` — groups seven under a literal `// Universal filter fields`
  comment, including `TargetBranch`.
- `config/load.go:195-211` — enforces config's version at load time.

Three definitions of "universal", none matching. Whoever writes `IssueFilterSpec` has a
coin flip.

**Nuance:** `State` is arguably *not* universal in the same way. The field name is
shared but the value domain isn't — `open|closed|merged|draft` for PRs,
`open|closed` for issues. Ties back to Q1.

**Also relevant:** the split shares *fields* but not *behavior*. Every predicate
constructor is hard-typed `Predicate[model.PullRequest]`, and Go methods can't take
type parameters, so a second resource type copies five constructors verbatim. Genuine
sharing needs free generic functions over an accessor interface or extractor funcs —
which is the prerequisite work CLAUDE.md already flags, not a free extension.

**Decision — universal is `Author`, `Repo`, `Provider`, `Label`, `StalenessGTE`,
`TargetBranch`. `State` is per-resource.**

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

Follows from this:

- `Label`, `StalenessGTE`, `TargetBranch` move from `PRFilterSpec` into
  `BaseFilterSpec`.
- **`State` stays per-resource**, and Q1 makes that principled rather than arbitrary:
  the field name is shared but the value domain isn't — PRs are
  `open|closed|merged`, issues would be `open|closed`. ADR-000 Layer 3 listing `State`
  as universal is the error.
- `config/load.go:171` moves `state` validation into a per-resource switch;
  `config/load.go:195`'s resource-incompatibility block gains `state` as a
  value-domain check rather than a presence check.
- ADR-000 Layer 3 and the `// Universal filter fields` comment at `config/config.go:70`
  both need correcting to this set.

Rejected: flattening `BaseFilterSpec` away until a second resource type exists (the
embed provides zero code reuse today and costs every construction site
`PRFilterSpec{BaseFilterSpec: BaseFilterSpec{...}}`, but reversing an ADR-000
commitment and re-adding it later is churn); doing the generic pass now (designing an
accessor interface with no second resource type to validate it against).

**Deferred, not rejected:** the generic pass itself. Record on `BaseFilterSpec` that
its predicates are PR-typed and that a second resource type triggers the work.

---

## Q6 — Where does `ResolvedIdentities` live?

**Evidence**

- `query/filter.go:15` — declared in `query`.
- ADR-009 §2 — identity resolution is an assembly-layer responsibility; "the facade's
  only contribution is the resolved identity map".
- ADR-000 Layer 4 — hook filter expressions reuse the `FilterSpec` vocabulary.
- ADR-000 §Layer boundary rules — the Event Engine is constrained to "the resource model only", so
  `event` will need to resolve `"me"` in hook filters and won't be permitted to name
  `query.ResolvedIdentities`.
- `config/config.go:104` — `HookConfig` already carries a `FilterConfig`.

Legal today (ADR-000's layer boundary rules let assembly import query), but it blocks a
planned consumer.
Moving it to `model` costs one line.

Related, same type: it's `map[string]model.Author`, but it's consumed by *reviewer*
matching as much as author matching (`query/filter.go:129`). `model.Author` is an alias
for `Identity`, so `map[string]model.Identity` is a zero-behavior rename.

**Decision — move to `model`, hold `Identity`, and clone in `Compile`.**

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

Follows from this:

- The future `event` import works without violating ADR-000's "resource model
  only" Event Engine boundary rule. `config.HookConfig` already carries a `FilterConfig`
  (`config/config.go:104`), so the need is visible in the code today even though
  `event/` is an empty stub.
- Value type becomes `model.Identity` — an alias, so zero behavior change, but the map
  resolves reviewers as much as authors (`query/filter.go:129`).
- `maps.Clone` makes the compiled predicate immutable, so ADR-006's "recompile when
  `resolvedMe` becomes available" (§ Bubble Tea integration) is enforced rather than
  merely documented. Without it, assembly writes into the map on identity recovery
  while the TUI reads it per keystroke — the same shape as the race fixed in `6b7512d`.
- Pairs with `LazyField` from Q2: `model` owns the vocabulary that crosses layers.
- The parameter should be renamed off `resolvedMe` at the same time (see deferred list)
  — it names a map as though it were a single value, and `resolveMe(raw, pullRequest,
  resolvedMe)` differs from its own function name by one character.

---

## Deferred — not design questions, just work

Tracked here so they aren't lost. None of these need a decision.

- **Enum vocabularies duplicated across packages.** `config/load.go:172,180,188` holds
  the same allowed-value lists as the switches at `query/filter.go:241-284`, including
  the "must be one of" prose. Four coordinated edits across two packages per new value.
  Fix: move the three parsers down into `model`, next to the enums they produce.
- **No `config.FilterConfig` → `query.PRFilterSpec` mapper exists.** Two structs
  describing the same TOML table, in two packages, nothing binding them — which is why
  Q5's drift went unnoticed.
- **`matchesStalenessAtLeast` is 0% covered**, because `time.Since` is called inline
  (`query/filter.go:212`). Extract `isStaleAtLeast(updatedAt, now, days)` and test that.
  Do *not* pass `now` into `Compile` — a compiled view lives for a session, and
  staleness would stop advancing. Also undocumented: the truncation at `:213` means a
  PR updated 71 hours ago does not satisfy `staleness_days = 3`.
- **Case-sensitivity is untested at all seven comparison sites**, including the
  deliberate exact match at `:105`. A contributor "fixing the inconsistency" breaks a
  documented decision with a green suite.
- **`target_branch` is a literal substring match**, so ADR-006 §1's `release/*`
  example doesn't work. It also lowercases (`:218`, `:220`), but git refs are
  case-sensitive. Either the example goes or the implementation owes a glob.
- **`repo = ""` becomes `[""]`** via `config.StringSlice` (`config/config.go:113`),
  passes the `len(...) > 0` guard at `filter.go:48`, and compiles to a match-nothing
  predicate. Same for `label`. Reject in `validate`. (Q3 covers the `provider` typo
  case; this one is separate.)
- **Negative `staleness_days`** is treated as "no filter" by the `> 0` guard at
  `filter.go:77` rather than as an error.
- **`status` vs `state` naming.** `model` uses both with distinct meanings (`CIStatus`
  the struct, `CIState` the enum); `query/filter.go:142,227,240,272` use `status` for
  `...State`-typed values. Worst inside `matchesCIState`, which holds `status`,
  `actualState`, and `ciStatus` in four lines. The spec fields `ReviewStatus`/`CIStatus`
  should stay — they mirror TOML keys frozen by ADR-005/ADR-006. Worth a comment saying
  so, or someone will "fix" them.
- **`resolvedMe`** names a map as if it were one value, and
  `resolveMe(raw, pullRequest, resolvedMe)` differs from its own function name by one
  character. Rename to `identities` with Q6's move. Also `raw` (`:101`) →
  `configuredUsername`, and `StalenessGTE` → `MinStalenessDays` (`GTE` is an
  abbreviation the convention forbids, and the TOML key is already `staleness_days`).
- **`resolveMe` takes a whole `model.PullRequest` but reads one field** — 
  `pullRequest.Provider.Name` (`:105`). Taking `instanceName string` makes the entire
  "me" seam resource-agnostic for free, ahead of Q5's deferred generic pass.
- **`Compile` returns on the first parse error** (`:57`, `:64`, `:83`), so a spec with
  three bad values is fixed one at a time. `errors.Join` is already the house pattern
  (`adapter/github/github.go:158`). Matters most for the wire API, where a client
  submits a whole spec at once.
- **Provider-side pushdown** (ADR-006 §The `"me"` sentinel) is worth its own ADR before the assembly
  layer hardens — it changes `ProviderAdapter`'s signature, and deciding after GitLab
  and Gitea implement against it is more expensive. Today an `author = "me"` filter
  fetches every open PR in every repo every 60s and discards nearly all of them.

---

## Q7–Q9 — surfaced while amending the ADRs, decided after

These came out of the ADR amendment pass; none was among the original six. Q8 and Q9
turned out to be one decision: giving `none` its own value makes the zero value mean
"not computed", which is the signal Q9 needed, so the cross-field coupling disappears.
Q7 then became answerable, because grouping can finally distinguish the two cases.

### Q7 — What bucket does a lazy field group into?

ADR-010 §2 is a *filter* rule. Grouping has no analogue: "unknown matches" means nothing
when every row must land in exactly one bucket.

`review_status` is a group key (ADR-006 §5, ADR-000 Layer 3). `query/group.go:88-91`
maps an empty `AggregateState` to a literal `"none"` group — so PRs whose reviews have
not loaded pile in with genuinely-unreviewed ones. That is the same conflation Q2d just
fixed on the filter side, unfixed on the grouping side.

**Decision — unresolved rows get their own bucket, and group keys report lazy-field
dependencies the way filters do.**

Two parts:

1. **`GroupSpec` reports its lazy-field dependency**, the same mechanism ADR-010 §2a
   gives `Compile`. A consumer that calls `EnsureLoaded` therefore has the data before
   it renders, which makes the bucket rare rather than routine.
2. **Anything still unresolved gets a distinct group**, rendered last — not folded into
   an existing one.

```
▾ changes requested (2)
▾ review required (5)
▾ none (1)
▾ not loaded (3)     ← distinct, last, usually empty
```

What follows:

- `query/group.go:88-91` currently maps an empty `AggregateState` to the `"none"`
  group. Q8's decision makes `""` mean "not computed", so grouping can finally tell the
  two apart — and folding them together would reintroduce, one layer up, the exact
  conflation Q8 removes.
- Consistent with ADR-010 §2's principle: unknown is *visible*, never disguised as a
  real answer. A PR with three approvals must not sit under a header reading `none`.
- The bucket's key needs a name that cannot collide with a real
  `AggregateReviewState`. `"none"` is now taken; `""` is the sentinel. Something like
  `not loaded` as a display label with its own key.
- `GroupSpec.ValidateForResource` already exists (`query/group.go:27`), so `GroupSpec`
  is an established home for this kind of method.

Rejected: the bucket without the prefetch mechanism (smaller, still honest, but a
grouped view would show one large "not loaded" pile on first paint — accurate and
useless); suppressing unresolved rows until they resolve (rows appearing and
disappearing under a grouped header is worse than a visible bucket).

**Open, minor:** whether `Diff` and `CI` need the same treatment. Neither is a group key
today (`query/group.go:13-17` lists `none`, `repo`, `provider`, `author`,
`review_status`), so `review_status` is the only lazy group key that exists. The
mechanism should be general even though only one key uses it.

### Q8 — `AggregateReviewStateNone` diverges from ADR-004

ADR-004 §Review state vocabulary specifies `AggregateReviewNone = "none"`.
`model/review.go:19` has `AggregateReviewStateNone = ""` — the zero value.

So "computed as none" and "never computed" are the same value, which is why
`review_status = "none"` matches a PR whose aggregate was never populated, including one
approved by three reviewers with no outstanding requests. `TestCompile_ReviewStatus_None`
locks in the current behavior with a PR that has no reviews at all, so it does not catch
this.

The ADR is right and the implementation regressed.

**Decision (with Q9) — `AggregateReviewStateNone = "none"`; the zero value `""` means
"not computed".**

```go
AggregateReviewStateNone AggregateReviewState = "none" // computed: no reviews
// "" is not an enum member. It is the zero value and means "not yet computed".
```

The two questions are one decision. Once `none` has its own value, `""` becomes an
unambiguous "not computed" marker — which is exactly the signal Q9 needed, and it lives
on the field itself rather than on a neighbouring one.

What follows:

- `model/review.go:19` sets the constant to `"none"`, matching what ADR-004 §Review
  state vocabulary already specifies.
- `adapter/github/normalize.go:119` keeps leaving the field at `""` when there are no
  requested reviewers — that is now *correct and meaningful*, not an oversight. It
  claims nothing until reviews load.
- `ComputeAggregateReviewState` returns `"none"` for the genuinely-no-reviews case, so
  a computed none and an uncomputed one are finally distinct.
- **The Q2c/Q2d rule for `review_status` reads one field only:**

  ```go
  if pullRequest.Reviews.AggregateState == "" {
      return true // not computed → unknown → match
  }
  return pullRequest.Reviews.AggregateState == wantedState
  ```

  No cross-field read of `ReviewerStates`'s load state. This resolves the objection
  that the model did not express the coupling — it now doesn't need to.
- A derived `review_required` is **compared, not passed through**, so a PR known to
  need review no longer matches `review_status = "approved"`. ADR-006 §2's "reliable
  lower bound" is honored rather than discarded.
- `review_status = "none"` stops matching PRs whose aggregate was never populated —
  the original defect. `TestCompile_ReviewStatus_None` pins the old behavior and must
  be updated.

Rejected: `AggregateState LoadResult[AggregateReviewState]` — it expresses "not
computed" in the type rather than as a zero value and would make `review_status` use
the identical rule as `ci_status`, but it is a `ReviewSummary` model change touching
every read site, and the derived-vs-fully-computed distinction would still need a home.
Worth revisiting if a second field develops the same shape. Also rejected: keeping
`None = ""` and guarding on `ReviewerStates.IsPending()`, which leaves Q8 unfixed and
discards the derived value.

**Note the asymmetry with `ci_status`, and that it is deliberate.** `CI` uses
`LoadResult`, so its unknown state is `IsPending() || IsError()`. `AggregateState` uses
`""`. Both implement ADR-010 §2's "unknown matches, known compares" rule; they differ
only in how each field spells "unknown". Worth a comment at both call sites so the
difference doesn't read as an oversight.

### Q9 — A derived `review_required` is known, not unknown

Q2c says `matchesAggregateReviewState` gains the same `IsPending()` guard as
`matchesCIState`. Two problems with the direct translation:

1. `AggregateState` is a plain enum, not a `LoadResult` (`model/review.go:80`), so the
   guard has to read `Reviews.ReviewerStates`'s load state while comparing a different
   field. The model does not express that coupling.
2. ADR-004 deliberately populates `AggregateState` *before* `ReviewerStates` loads,
   deriving `review_required` from requested reviewers, and ADR-006 §2 calls it a
   reliable lower bound for that one value. A blanket "pending → match" discards it: a
   PR known to be `review_required` would match `review_status = "approved"`.

**Decided together with Q8 — see the Q8 decision above.**

Giving `none` its own value makes `""` mean "not computed", which collapses the rule to
a single-field check and removes the cross-field coupling entirely:

```
AggregateState == ""  → unknown → match
otherwise             → compare
```

A derived `review_required` falls into "otherwise" and is compared, which is what this
question was asking for. No `LoadResult` wrapper and no read of `ReviewerStates`'s load
state is needed.

---

## Stale ADR text found along the way

- ADR-006 §Startup: resolving `"me"` sentinels — "filters using `me` for that provider
  are **disabled** with a startup warning", which reads as pass-through. The code
  correctly implements ADR-009 §3 (match-nothing) instead.
- ADR-006 §Startup: resolving `"me"` sentinels — still describes identities as
  `map[ProviderKind]ResolvedIdentity`. STE-74 added the superseded note under
  ADR-006 §Go predicate type but left this sentence.
