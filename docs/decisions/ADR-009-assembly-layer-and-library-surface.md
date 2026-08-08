# ADR-009: Assembly Layer and Library Surface

## Status

Accepted

## Context

ADR-008 decoupled adapter constructors from `config.ProviderConfig` and asserted that "the assembly layer (`client/`) owns the mapping." That clause named a package whose purpose had never been decided. `client/` was created as an empty scaffold directory in STE-54, and `docs/decisions/research/project-structure.md` lists its placement as "Decision needed before scaffolding." ADR-008 closed that open question by citation, inside an ADR whose subject was adapter constructor inputs.

Two documents were left in conflict, and ADR-000 also contradicted itself:

- ADR-000 Layer 5 stated: *"Each consumer adds a thin assembly and transport layer."* That reads as per-consumer assembly, with no shared package.
- ADR-000's own Consequences stated: *"the MCP server and HTTP API do not require a separate data pipeline. They compose the same layers the TUI uses."* A pipeline that four consumers share is a package.

Meanwhile the work between "config parsed" and "consumer renders" was undefined and homeless. `internal/poller/` existed as a one-line stub, giving that work two candidate homes.

### What assembly actually involves

The jobs between a parsed config and a rendered list are not glue. They are product behavior that must be identical across consumers, or prsm becomes several different products wearing one name:

- **Partial-failure semantics.** `adapter/github` already establishes the contract *within* one adapter: return the repos that succeeded, join the errors from the rest. Something must do the same *across* adapters. If the TUI, MCP server, and HTTP API each decide independently what "GitLab is down" means, the multi-provider commitment is honored nowhere.
- **Liveness.** ADR-003 specifies rate-limit backoff to `X-RateLimit-Reset`, offline marking after consecutive failures, a startup burst that bypasses the interval, and per-provider staleness indicators. That is a specification of behavior, not of rendering.
- **Identity resolution.** Compiling any filter containing a `"me"` sentinel requires resolved identities for every provider. Producing them means calling `ResolveIdentity` on each adapter and deciding what a partial failure means.

## Decision

### 1. A single shared assembly layer, at the module root

The assembly layer is `package prsm` at the module root — `github.com/lstellway/prsm` — exposing `prsm.New(cfg) (*prsm.Client, error)`. Consumers add only transport or presentation over it.

`client/` is removed as the assembly layer's home (see §4). `internal/poller/` is removed; its job belongs to the facade.

Root placement was previously rejected in `research/project-structure.md` on the grounds that "the root package would import all the provider SDK dependencies." That reasoning does not hold: any facade constructing all three adapters imports all three SDKs wherever it sits. Module requirements are module-wide regardless, and Go compiles only imported packages — a consumer importing `prsm/model` alone pulls in no provider SDK under either layout.

### 2. Responsibility split

Owned by the assembly layer:

- Mapping `config.ProviderConfig` to each adapter's `Config` (the substance of ADR-008)
- Dispatching on `ProviderConfig.Type` and constructing adapters
- Resolving `"me"` identity sentinels at startup
- Fanning out `ListPullRequests` across providers, with per-adapter timeouts and context cancellation
- Aggregating partial failures into a structured result
- Driving the refresh/poll cycle per ADR-003, including backoff and offline marking
- Holding the last-known-good snapshot in memory, for offline display and as the substrate ADR-007 diffs against
- Routing a lazy-load request back to the adapter that owns the pull request
- Constructing the Event Engine and exposing its stream (v1.1, ADR-007)

Owned by each consumer:

- **When** to trigger lazy loads. The mechanism is shared; the policy is not. The TUI fetches on cursor movement, an HTTP consumer on a query parameter, MCP on a tool argument. Centralizing the trigger would force every consumer into the TUI's cursor semantics.
- Applying `query.Apply`. It is a pure function over a slice and the TUI re-runs it per keystroke; round-tripping interactive filtering through the facade would be wrong. The facade's only contribution is the resolved identity map.
- All transport and rendering. The assembly layer must not import a UI or transport framework.

> **Amended by ADR-010 §2.** Assembly gains one responsibility — `EnsureLoaded`, a mechanism that completes a given set of lazy fields across the current snapshot:
>
> ```go
> // package prsm
> func (client *Client) EnsureLoaded(ctx context.Context, fields []model.LazyField) error
> ```
>
> **The policy split above is unchanged.** *When* to complete lazy fields is still each consumer's decision — the bullet at :52 stands verbatim. The TUI calls it on view activation, MCP per request, an HTTP consumer on a query parameter. What changes is that assembly now offers a batch mechanism alongside the per-request routing at :47, so that every consumer completes data the same way instead of each inventing its own. ADR-010 §2 has `Compile` report the lazy fields a predicate depends on; `EnsureLoaded` is the thing that acts on that report.
>
> Sequence for a one-shot consumer filtering on `ci_status = "failing"`:
>
> ```
> MCP client       MCP server        prsm.Client       GitHub adapter    GitHub API
>     │ list_prs       │                   │                 │               │
>     ├───────────────>│                   │                 │               │
>     │                │  Snapshot()       │                 │               │
>     │                ├──────────────────>│                 │               │
>     │                │<─ ─ ─ ─ ─ ─ ─ ─ ─ ┤  50 PRs, CI Pending             │
>     │                │                   │                 │               │
>     │                │ Compile() ──> predicate, needs["ci"] │               │
>     │                │                   │                 │               │
>     │                │ EnsureLoaded(["ci"])                 │               │
>     │                ├──────────────────>│  LoadCI(pr) ×50 │               │
>     │                │                   ├────────────────>├──────────────>│
>     │                │                   │<─ ─ ─ ─ ─ ─ ─ ─ ┤<─ ─ ─ ─ ─ ─ ─ ┤
>     │                │<─ ─ ─ ─ ─ ─ ─ ─ ─ ┤  written back into the snapshot │
>     │                │ Apply()           │                 │               │
>     │<───────────────┤                   │                 │               │
>     │ 3 PRs, correct │                   │                 │               │
> ```
>
> Three requirements are load-bearing, not implementation latitude:
>
> - **Write-back is required, not an optimization.** Results are stored into the snapshot assembly already holds (:46). A per-request throwaway load costs one API call per pull request on every request and will exhaust the ADR-003 budget. With write-back the first call pays and the 60-second poll refreshes via ETags for free.
> - **Loads are bounded and concurrent.** `LoadCI` and `LoadReviewerStates` are per-pull-request calls (`adapter/github/github.go:206`, `:246`), so the work is one call per pull request per field. Roughly 50 pull requests at ten concurrent is about one second.
> - **There is a cap, and hitting it must be reported.** A filter over 500 pull requests is 500 calls. Silent truncation would read as "covered everything" — the caller must be able to tell a complete pass from a truncated one, for the same reason §3 requires that "no PRs" and "couldn't fetch" not look alike.
>
> **Why the mechanism belongs to assembly and not to each consumer.** §4 ("The two surfaces must stay semantically consistent") requires that a snapshot mean the same thing in-process and over the wire, and calls divergence between the two surfaces a defect. If `prsm serve` completes its lazy fields and the TUI does not, `ci_status = "failing"` returns different sets depending on which surface you ask — one query, two answers, both defensible locally. Sharing the mechanism is what makes that consistency enforceable; leaving completion to each consumer leaves it to convention.

### 3. Fetch results are structured, not `([]PullRequest, error)`

A fetch returns a snapshot carrying both the pull requests and per-provider status — name, kind, state (ok / offline / rate-limited), last success time, and error. ADR-003 requires surfacing per-provider staleness; every consumer needs that shape and only the rendering differs.

**A provider that fails is never fatal.** If a provider's identity cannot be resolved or its fetch fails, it is marked offline and the remaining providers serve normally. `"me"` matches nothing on an unavailable provider until it recovers. prsm does not refuse to start because one configured provider is unreachable — a single expired token on a secondary provider must not block the tool. Consumers are responsible for making degraded state visible, since "no PRs from this provider" and "this provider is unreachable" must not look alike.

> **Clarified against ADR-010 §2; behavior unchanged.** ADR-010 §2 rules that an unknown lazy field *matches* — a pull request whose CI or reviewer state is still pending, or whose load failed outright, passes a `ci_status` or `review_status` filter rather than being excluded. That is the opposite disposition from `"me"` matching nothing on an unavailable provider, and the two are deliberate because they are different axes.
>
> A lazy field is per-pull-request *data* that is not yet known. Excluding it would drop individual rows silently, with no provider-level signal to attribute the gap to — so the unknown row stays visible and is corrected when the value arrives.
>
> `"me"` is not data; it is the *filter term itself* failing to resolve. An unresolved sentinel has no login to compare against, so there is no comparison to defer — matching everything would silently reinterpret `author = "me"` as `author = anyone` for that provider, which is a different query, not a broader one. And unlike a per-pull-request load failure, the gap is attributable: the provider is marked offline in the same snapshot, and this section already requires consumers to surface that. Both rules refuse to compare against a value they do not have; they differ in what is missing.

### 4. Two first-class integration surfaces

Integrators are supported through two parallel official surfaces, both documented and versioned:

- **The Go library** — `package prsm`. The integrator embeds prsm's aggregation in their own process, with their own configuration and credentials.
- **The wire API** — `prsm serve`, defined by `api/proto/prsm/v1`. The integrator queries a prsm instance someone else is running, with its configuration and credentials.

`client/` is reserved for the hand-written Go SDK for the wire API. This is the second reason the assembly layer cannot live there: in a repository that ships a server, a package named `client/` is read as the client for that server. ("Client" survives as the facade's *type* name, `prsm.Client`, where it reads correctly.)

**The two surfaces must stay semantically consistent.** A snapshot retrieved over the wire means exactly what a snapshot means in-process, including the degraded-provider semantics in §3. Divergence between them is a defect, not an implementation detail.

### 5. Identity is keyed by provider instance, not provider kind

Resolved identities are keyed by `ProviderInstance.Name`, not `model.ProviderKind`. prsm explicitly supports several instances of the same kind — a github.com account and a GitHub Enterprise Server account, with different logins. Keying by kind collapses them to one identity, so `"me"` resolves incorrectly whenever two instances of one kind are configured.

Each pull request belongs to exactly one provider instance and is matched against that instance's identity. Union semantics across identities were rejected: they differ from per-instance matching only when a login collides across hosts, and in that case they are simply wrong.

> **Amended by ADR-010 §6.** The decision above stands unchanged — identities are keyed by `ProviderInstance.Name`. The *type* moves:
>
> ```go
> // model
> type ResolvedIdentities map[string]Identity // keyed by ProviderInstance.Name
>
> // query
> func (filterSpec PRFilterSpec) Compile(
>     identities model.ResolvedIdentities,
> ) (Predicate[model.PullRequest], []model.LazyField, error) {
>     identities = maps.Clone(identities)
>     ...
> }
> ```
>
> `ResolvedIdentities` is declared in `model` rather than `query`, and holds `model.Identity` rather than `model.Author` — an alias, so no behavior changes, but the map is consumed by reviewer matching as much as by author matching and the neutral name says so.
>
> **The move is forced by ADR-000's layer boundary rules** — "the Event Engine must not import consumer packages; it may import the resource model only." `event` will need to resolve `"me"` in hook filters, and `config.HookConfig` already carries a `FilterConfig`, so the need is visible in the code today even though `event/` is an empty stub. It cannot be permitted to name `query.ResolvedIdentities`. Declaring the type in `query` is legal now — the same rules let assembly import query — but it blocks a planned consumer for the price of one line.
>
> **`Compile` takes a `maps.Clone` of the map**, making a compiled predicate immutable with respect to later identity changes. Assembly writes into the map when a provider's identity recovers (§3), while a consumer reads it per keystroke through an already-compiled predicate — the same data race shape as the one fixed in commit `6b7512d`. The clone also turns ADR-006's "compiled once when the view activates or when `resolvedMe` becomes available" (§ Bubble Tea integration) from documentation into a property the code enforces: a stale predicate stays visibly stale rather than mutating underfoot.

### 6. Versioning

The module stays at v0.x until the TUI ships. Go treats v0 as an explicit absence of a compatibility promise, and the facade is currently designed against no real usage — the first consumer will teach us things. Both integration surfaces move to v1 together, not independently.

### 7. Adapter injection

`prsm.NewWithAdapters(...)` accepts pre-constructed adapters alongside `prsm.New(cfg)`. This is what actually realizes ADR-008's goal of adapters being constructible without the config layer, and it makes the facade testable against `adapter/mock`.

## Consequences

**Easier:**

- Liveness, partial-failure, and identity semantics are specified once and shared by every consumer.
- Library integrators get the poll loop and the event stream, which ADR-007 identifies as the primary reason to use prsm as a library.
- Adding a consumer means adding transport, not re-deriving behavior.
- `go doc github.com/lstellway/prsm` is the library's landing page.

**Harder — accepted costs:**

- **Two public API surfaces to maintain and keep consistent.** The library and the wire API are both official, both versioned, and must not drift semantically.
- **ADR-003's backoff and offline semantics become public behavior**, not an implementation detail that can be quietly changed.
- **The assembly layer imports every provider SDK.** Unavoidable in any shared facade, and it does not affect consumers importing only `model` or `query`.
- **The wire API is currently an empty skeleton.** `api/proto/prsm/v1/prsm.proto` defines no services or RPCs, `api/gen/` is empty, and there is no codegen configuration. Promoting the wire API to first-class makes this a tracked gap rather than a someday-thing.
- **Assembly must know how to complete every lazy field, for every adapter** (added by ADR-010 §2). It already routes per-request lazy loads (:47), so `EnsureLoaded` extends knowledge assembly has rather than introducing a new coupling — but each new lazy field on a resource type now lands in two places, the adapter that fetches it and the batch path that completes it. Concurrency limit, cap, and truncation reporting are assembly's to get right once, on everyone's behalf.
- **A consumer that skips `EnsureLoaded` gets an over-broad result, not a wrong-but-plausible one** (added by ADR-010 §2). Because unknown lazy fields match, skipping the mechanism returns too many rows — visibly broken, and the failure mode was chosen for exactly that reason. It remains a real cost: correctness of any filter over a lazy field depends on the consumer calling a mechanism nothing compels it to call. `Compile` reporting the dependency is the only guardrail; a consumer is free to ignore the returned set.

## Amendments to prior ADRs

- **ADR-008 item 4** — the substance stands; the package designation is withdrawn. The assembly layer owns the mapping and is the only package importing both config and adapters; its identity is decided here, not there.
- **ADR-000 Layer 5** — the per-consumer assembly wording is replaced by the shared assembly layer described above, resolving the contradiction with ADR-000's own Consequences section.

## Amendments to this ADR

- **§2, by ADR-010 §2** — assembly gains `EnsureLoaded`, a batch mechanism for completing lazy fields across the snapshot, with required write-back, bounded concurrency, and a reported cap. The responsibility split itself is unchanged: the mechanism is shared, the trigger stays with each consumer.
- **§3, by ADR-010 §2** — no behavior change. A clarifying note distinguishes unresolved identity (matches nothing) from an unknown lazy field (matches); the two rules govern different axes.
- **§5, by ADR-010 §6** — the keying decision is unchanged; `ResolvedIdentities` moves from `query` to `model`, holds `model.Identity`, and is cloned by `Compile`.

## References

- ADR-000: System Architecture — layer definitions and boundary rules
- ADR-003: Liveness Model — poll cadence, backoff, offline marking
- ADR-007: Event Engine — the library-facing event stream this layer exposes
- ADR-008: Adapter Constructor Inputs — the mapping whose home this ADR settles
- ADR-010: Filter Semantics — amends §2, §3, and §5 of this ADR
- `docs/decisions/research/project-structure.md` — the original open question
- `docs/decisions/research/filter-semantics-open-questions.md` — the evidence behind ADR-010's amendments (Q2, Q6)
