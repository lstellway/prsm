# ADR-008: Adapter Constructor Inputs and Config Mapping

## Status

Accepted

## Context

`adapter/github.New()` originally took a `config.ProviderConfig` directly. This made `adapter/github` import `github.com/lstellway/prsm/config`, which conflicts with ADR-000's layering: adapters are Layer 1 and must not depend on the assembly layer's input format. Two concrete symptoms:

- Adapter unit tests had to construct TOML-shaped config fixtures to exercise a constructor that only needed a token and a repo list.
- A future library, MCP, or HTTP consumer (ADR-000 Layer 5) could not construct an adapter without importing the config package and its TOML struct tags.

The cost of fixing this rises sharply once GitLab and Gitea/Forgejo adapters exist, since all three would replicate the pattern and need refactoring simultaneously. This ADR was therefore resolved before those adapters were implemented.

Two properties of the problem shape the available options:

1. **Provider constructor inputs genuinely differ.** GitLab supports group/namespace scoping. Gitea is the only provider accepting basic auth (ADR-002, ADR-005). GitHub needs neither.
2. **`config.ProviderConfig` is a union of every provider's fields.** It is the shape of the TOML file, not the shape of any single adapter's requirements.

### Options considered

**A. Per-adapter `Config` struct in each adapter package.** Each adapter declares only the fields it consumes; the assembly layer maps `config.ProviderConfig` into it.

**B. A shared `adapter.Config` struct used by all adapters.** Avoids repetition, but produces a union struct: every adapter imports a type carrying fields it must ignore, and a change to Gitea's auth model edits a type `adapter/github` depends on. That is the coupling this ADR removes, relocated one layer down. It also destroys the property that makes option A valuable — an adapter's `Config` stating its complete input contract in a few lines.

**C. A narrow constructor interface.** The input is pure data with no behavior, so an interface buys dynamic dispatch nobody consumes while forcing getter boilerplate. It does not solve field variance either: GitLab still needs groups, so it lands on a type assertion or a second interface. It also degrades constructor tests from struct literals to a fake type per case.

**D. Functional options** (`New(name string, opts ...Option)`). Options earn their keep with a wide optional surface, meaningful defaults, and hand-written call sites evolving under third parties. Here there are roughly five fields, set once, by exactly one mechanical caller. Options would trade compile-time field checking for runtime silence when a field is forgotten. Keyed struct literals already provide the additive evolution that options are usually bought for.

Placing the mapping in `config/` rather than the assembly layer was also rejected: it would make the config package import all three adapter packages — worse coupling than the original problem, untestable without the adapter tree, and provider dispatch in the layer whose job is parse-and-validate.

## Decision

1. **Each adapter package declares its own exported `Config` struct** containing only the fields that adapter consumes — `github.Config`, `gitlab.Config`, `gitea.Config` (option A).

2. **Constructors take that `Config` by value** and return `(*Adapter, error)`. This follows the standard-library convention for construction-parameter bags (`http.Server`, `tls.Config`, `net.Dialer`). All call sites use keyed literals, so adding a field stays source-compatible.

3. **No adapter package may import `github.com/lstellway/prsm/config`.** This extends ADR-000's layer boundary rules, which govern the four architectural layers but do not name the config package explicitly.

4. **The assembly layer owns the mapping** from `config.ProviderConfig` to each adapter's `Config`, and is the only package importing both sides. `config/` must not import adapter packages. The assembly layer's package identity is *not* decided by this ADR — see ADR-009.

5. **Boundary rule for the shared `adapter` package**: it holds the `ProviderAdapter` interface and types universal to all providers. Provider-specific reference types live in that provider's package. Accordingly `adapter.RepoRef` is shared — every provider polls owner/repo pairs — while `GroupRef` is GitLab-only and lives in `adapter/gitlab`.

6. **Constructors validate their own preconditions.** `config/` validation at load time does not cover adapters constructed directly by library or MCP consumers, so this is deliberate defence-in-depth rather than redundancy. Validation belongs in `New()`; there is no separate `Validate()` method, which would create a "must I call this?" ambiguity and a second path that can drift.

7. **Adapter errors use adapter vocabulary, not config-file vocabulary.** `New()` reports `token is required`, not `auth.token is required` — the TOML key path is the config layer's to name when it wraps the error.

## Consequences

**Easier:**

- Adapter tests construct plain struct literals and need no config fixtures; `adapter/github/constructor_test.go` shrank by roughly half.
- Each adapter's input requirements are self-documenting and independently evolvable — a change to Gitea's auth model touches no other adapter.
- Adding a provider is one `Config` struct plus one mapping function, with no edits to shared types.
- Adapters are constructible by any consumer without importing the config layer, which is a precondition for the library and MCP consumers in ADR-000 Layer 5.

**Harder — accepted costs:**

- **N mapping functions to maintain**, one per provider, in the assembly layer.
- **`adapter.RepoRef` deliberately duplicates `config.RepoRef`.** The two are permitted to diverge; converting between them is the unavoidable tax of the layering rule. It is paid once, in the assembly layer's `toRepoRefs`.
- **Config-to-adapter drift is not compiler-checked.** A field added to `config.ProviderConfig` and mapped nowhere compiles and ships silently. Mapping tests in the assembly layer are therefore a required mitigation, not optional: they are the only place the field-by-field correspondence is asserted.
- **Provider-incompatible config fields drop silently.** `config.ProviderConfig` is a union and `config/load.go` does not validate type-to-field compatibility, so `[[providers.groups]]` under a `type = "github"` provider yields a clean startup and an empty result set with no diagnostic. Validating that compatibility in `config/load.go` is a follow-up obligation of this decision.
- **`Auth.Type` is currently carried by no mapper.** `config/` validates `auth.type` against `pat | oauth | basic`, but the adapter `Config` types re-derive auth mode from which credential fields are populated. Reconciling the declared auth type with the derived one is a follow-up, and must be resolved when the Gitea adapter lands, since Gitea is the only provider with two auth modes.

**Revisit if:** adapter constructors become public API consumed by third parties outside this repository. At that point the forward-compatibility argument for functional options (option D) strengthens considerably. Moving from a struct to options is a breaking change, but a mechanical one, and the struct does not foreclose it.

## References

- ADR-000: System Architecture — layer definitions and boundary rules
- ADR-002: v1 Provider Set — why the three providers' auth and scoping models differ
- ADR-005: Config Format — `config.ProviderConfig` and the auth types it validates
- ADR-009: Assembly Layer and Library Surface — decides where the mapping lives, amending item 4 above
- Linear STE-68 — the originating issue
