# Multi-Resource FilterSpec Design

## What is hard to change

**The TOML key namespace is a public API.** Once users have `[views.filter]` configs with `reviewer`, `review_status`, `ci_status`, and `target_branch`, those key names cannot be renamed without breaking their configs. That is the actual hard constraint — not the Go struct shape.

**The `FilterSpec` → `Predicate[T]` compilation boundary.** The decision of what type `T` is in `Predicate[T]` is baked into every call site that calls `.Compile()`. If `T` is `PullRequest` today and becomes an interface or union later, every place that constructs or consumes a predicate must be updated. This is the refactoring surface to minimize.

**Struct field names in the TOML decoder.** The TOML library maps `[views.filter]` keys to struct fields by name (or `toml` tags). If `PRFilterSpec.Reviewer` and `IssueFilterSpec.Assignee` are two separate struct types, a `[views.filter]` section with both `reviewer` and `assignee` keys cannot be decoded into a single struct without one of them being ignored or erroring. This matters because prsm currently decodes one `[views.filter]` table per view — the decoder needs to know the concrete target type.

**What is not constrained:** the Go struct shape itself, field grouping via embedding, and which fields are zero-valued vs. present. These are purely internal and can be refactored freely as long as the TOML key names stay stable and the `Compile` signature stays compatible.

## Recommendation: Option B (shared base + resource-specific extension), deferred until Issues are scoped

For v1, do nothing new. Ship `FilterSpec` exactly as ADR-006 defines it for PRs.

When Issues are added, use this shape:

```go
type BaseFilterSpec struct {
    Author       string
    Label        []string
    Repo         []string
    Provider     []string
    StalenessGTE int
    State        string
}

type PRFilterSpec struct {
    BaseFilterSpec
    Reviewer     string
    ReviewStatus string
    CIStatus     string
    TargetBranch string
}

type IssueFilterSpec struct {
    BaseFilterSpec
    Assignee  string
    Milestone string
}
```

**Why not Option A (two fully independent structs)?** It works, but shared logic (author, label, repo, provider, staleness filtering) must be duplicated or factored into a helper that takes the common fields as parameters. That duplication is an ongoing maintenance cost, not a one-time cost. Embedding avoids it cleanly.

**Why not Option C (interface)?** `FilterSpec interface { Compile(...) Predicate[Resource] }` requires a `Resource` union type or interface that both `PullRequest` and `Issue` satisfy. That means defining a `Resource` interface with a common method set, which requires the data model layer to commit to what PR and Issue share before Issue is designed. That is over-engineering before Issues are scoped. The predicate is already generic — the compilation step is where the resource type is introduced, and keeping that in concrete `Compile` methods on concrete structs is simpler and equally type-safe.

**Why embedding works in Go:** `toml.Decode` with a struct pointer walks embedded fields as if they were promoted to the outer struct. `author`, `label`, `repo`, `provider`, `staleness_gte`, and `state` in a `[views.filter]` TOML table decode into `BaseFilterSpec` fields transparently. `reviewer`, `review_status`, etc. decode into the outer `PRFilterSpec` fields. No ambiguity — TOML keys map to struct fields, and a key that does not exist on the target type is either ignored or errors depending on decoder config. Setting `DisallowUnknownFields` per view type means an `assignee` key in a PR view's filter table is caught at config load time.

**The TOML namespace question:** `reviewer` belongs to PR views; `assignee` belongs to Issue views. Both live under `[views.filter]`. This is not ambiguous because a view is typed — the config will need a `type = "pr"` or `type = "issue"` field on the view definition, and the decoder selects the appropriate `FilterSpec` subtype. The key names do not collide within a single view's filter table. If a user accidentally puts `assignee` in a PR view's filter, strict decoding rejects it.

**The `Predicate[T]` calculus:** The generic runtime predicate is already correct. `PRFilterSpec.Compile(...) Predicate[PullRequest]` and `IssueFilterSpec.Compile(...) Predicate[Issue]` are two separate methods returning predicates over different concrete types. No `Resource` union is needed. The Bubble Tea model holds a `compiledFilter Predicate[PullRequest]` for the PR list view and (eventually) a `compiledFilter Predicate[Issue]` for the issue list view. These are separate model instances for separate view kinds. The generic type parameter is resolved at compile time per view kind — this is exactly what generics are for.

## What is NOT hard to change

- Renaming Go struct fields (as long as `toml` tags keep the public key names stable).
- Extracting `BaseFilterSpec` from `FilterSpec` later — this is a pure refactor with no behavioral change and no config impact.
- Adding new filter keys to either spec — TOML keys are additive; old configs without the new key just get the zero value.
- The `Compile` method signature — only called in the startup path, easy to update in one place.
- Whether `BaseFilterSpec` is an embedded struct or a separate helper function — both are internal details.

## Suggested ADR-006 addition

Add to the Consequences section:

> **Multi-resource extensibility:** `FilterSpec` is intentionally PR-specific in v1. When a second resource type (e.g., Issue) is added, `FilterSpec` will be refactored into `BaseFilterSpec` (fields shared across resource types: `Author`, `Label`, `Repo`, `Provider`, `StalenessGTE`, `State`) and `PRFilterSpec` / `IssueFilterSpec` extending it via Go embedding. The TOML key names for shared fields (`author`, `label`, `repo`, `provider`, `staleness_days`, `state`) are stable public API and must not change during this refactor. `Predicate[T]` already supports this pattern: `PRFilterSpec.Compile()` returns `Predicate[PullRequest]`; `IssueFilterSpec.Compile()` will return `Predicate[Issue]`. No `Resource` union type is required. View configs will need a `type` discriminator field to select the correct `FilterSpec` subtype at decode time.
