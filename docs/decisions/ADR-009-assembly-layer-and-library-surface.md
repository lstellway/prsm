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

### 3. Fetch results are structured, not `([]PullRequest, error)`

A fetch returns a snapshot carrying both the pull requests and per-provider status — name, kind, state (ok / offline / rate-limited), last success time, and error. ADR-003 requires surfacing per-provider staleness; every consumer needs that shape and only the rendering differs.

**A provider that fails is never fatal.** If a provider's identity cannot be resolved or its fetch fails, it is marked offline and the remaining providers serve normally. `"me"` matches nothing on an unavailable provider until it recovers. prsm does not refuse to start because one configured provider is unreachable — a single expired token on a secondary provider must not block the tool. Consumers are responsible for making degraded state visible, since "no PRs from this provider" and "this provider is unreachable" must not look alike.

### 4. Two first-class integration surfaces

Integrators are supported through two parallel official surfaces, both documented and versioned:

- **The Go library** — `package prsm`. The integrator embeds prsm's aggregation in their own process, with their own configuration and credentials.
- **The wire API** — `prsm serve`, defined by `api/proto/prsm/v1`. The integrator queries a prsm instance someone else is running, with its configuration and credentials.

`client/` is reserved for the hand-written Go SDK for the wire API. This is the second reason the assembly layer cannot live there: in a repository that ships a server, a package named `client/` is read as the client for that server. ("Client" survives as the facade's *type* name, `prsm.Client`, where it reads correctly.)

**The two surfaces must stay semantically consistent.** A snapshot retrieved over the wire means exactly what a snapshot means in-process, including the degraded-provider semantics in §3. Divergence between them is a defect, not an implementation detail.

### 5. Identity is keyed by provider instance, not provider kind

Resolved identities are keyed by `ProviderInstance.Name`, not `model.ProviderKind`. prsm explicitly supports several instances of the same kind — a github.com account and a GitHub Enterprise Server account, with different logins. Keying by kind collapses them to one identity, so `"me"` resolves incorrectly whenever two instances of one kind are configured.

Each pull request belongs to exactly one provider instance and is matched against that instance's identity. Union semantics across identities were rejected: they differ from per-instance matching only when a login collides across hosts, and in that case they are simply wrong.

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

## Amendments to prior ADRs

- **ADR-008 item 4** — the substance stands; the package designation is withdrawn. The assembly layer owns the mapping and is the only package importing both config and adapters; its identity is decided here, not there.
- **ADR-000 Layer 5** — the per-consumer assembly wording is replaced by the shared assembly layer described above, resolving the contradiction with ADR-000's own Consequences section.

## References

- ADR-000: System Architecture — layer definitions and boundary rules
- ADR-003: Liveness Model — poll cadence, backoff, offline marking
- ADR-007: Event Engine — the library-facing event stream this layer exposes
- ADR-008: Adapter Constructor Inputs — the mapping whose home this ADR settles
- `docs/decisions/research/project-structure.md` — the original open question
