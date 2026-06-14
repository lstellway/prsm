# Multi-Resource Query Layer: What Is Hard to Change

## What Is Hard to Change

### 1. Config file strings are a public API

`by = "review_status"` in a user's TOML file is the hardest thing to change in the current grouping model. Once a user has written that string into their config and it works, renaming it or removing it breaks their setup silently or with a confusing error. This is a compatibility contract from day one — every string value that ships in a stable release is a migration obligation.

The current enum (`"none" | "repo" | "provider" | "review_status" | "author"`) mixes two different scopes: `review_status` is PR-specific; the others are universal. When Issues land, a config with `by = "review_status"` under an Issue view is either invalid or has an undefined fallback. The spec does not define which. That undefined behavior is what will become hard to change — not the enum values themselves, but the behavioral contract around invalid values.

**The actual hard constraint:** You must decide now what an invalid grouping key does in context, and you must document it as a stable behavior. Either it is an error (schema validation at load time, not at render time) or it is a silent `"none"` fallback. Whichever you pick becomes load-bearing for config compatibility.

### 2. FilterSpec is typed to PullRequest

`FilterSpec` is a concrete struct with fields like `ReviewStatus string` and `CIStatus string`. This is fine for v1. The hard part: when you add Issues, you either duplicate FilterSpec into `IssueFilterSpec` (two compile paths, two Compile methods, two TOML tables) or you generalize it. Generalizing a concrete struct after users have written config against it requires either a breaking schema change or keeping the old field names while adding new semantics — both are ugly. The time to make this decision is before the first stable release, not after.

### 3. Sort key strings, same problem

`by = "updated"` is safe and universal. If a future sort key like `"ci_status"` ships in v1 and is later revalued, config breaks. The specific risk is adding resource-specific sort keys (like `"priority"` for Issues) into the same `[views.sort]` table without a way to know which resource type the view applies to. A sort key that is valid in a PR view but invalid in an Issue view has the same contract problem as grouping.

---

## Recommendation

### Do now

**Add a `resource` field to named view definitions.** Every named view should declare which resource type it applies to: `resource = "pr"` (default for v1, backward-compatible). This is the minimal anchor that makes everything else tractable. With it:
- Grouping and sort key validation becomes resource-scoped at config load time.
- A view with `resource = "pr"` and `by = "review_status"` is valid. The same grouping in a view with `resource = "issue"` is a load-time error with a clear message.
- When Issues ship, `resource = "issue"` views get their own valid grouping and sort key space without touching the PR config.

**Define the invalid-key behavior now as schema validation, not silent fallback.** Silently falling back to `"none"` when a key is invalid hides bugs and makes config feel unreliable. Emit an error at startup with the offending key and the valid alternatives. This is more debuggable and sets the right expectation: config is validated, not guessed at.

**Keep the grouping key namespace flat but resource-scoped in validation.** Do not create separate `[views.pr_group]` and `[views.issue_group]` TOML tables. The key `by = "review_status"` stays a string; the validation of which strings are legal is gated by the view's `resource` field. This keeps the config schema simple and the implementation straightforward.

### Defer

**Do not design a shared FilterSpec generalization now.** The shape of Issue filters is unknown. A premature `GenericFilterSpec` with interface fields will be worse than duplicating the struct when the time comes. The right move is: when Issues land, add `IssueFilterSpec` as a parallel type with its own `Compile()` → `Predicate[Issue]` method. Share the `Predicate[T]` type (already generic) and the composition helpers (`And`, `Or`). Do not share the spec struct.

**Do not add Issue-specific grouping keys to the enum now.** `"milestone"` and `"assignee"` do not exist yet. Define them when Issues are specced. The `resource` field on the view definition gives you the isolation to add them without ambiguity.

---

## What Is NOT Hard to Change

**The Predicate[T] type.** It is already generic (`func(T) bool` with `And`/`Or` combinators). Adding `Predicate[Issue]` alongside `Predicate[PullRequest]` requires zero changes to the type definition. This is the one part of the current design that is already multi-resource-ready.

**The universal grouping keys.** `repo`, `provider`, `author`, `none` apply to any resource type. Extending the valid-key set for a specific resource type later does not affect these. They are safe.

**The sort keys** `updated`, `created`, `staleness`, `title`. These fields exist on any time-tracked resource. If an `Issue` type has `UpdatedAt`, `CreatedAt`, and `Title`, these sort keys are valid for it without any schema change.

**The Bubble Tea integration specifics.** The `compiledFilter Predicate[PullRequest]` in the TUI model is a rendering concern. Switching it to `Predicate[Issue]` when rendering an Issue view is a mechanical substitution. The hard part is not how the TUI holds the predicate — it is how the config schema describes which resource a view applies to.

**The fuzzy `/` session filter.** It operates on a composite string built from whichever fields make sense for the resource being displayed. It is already decoupled from FilterSpec. No changes needed.

---

## Suggested ADR-004/006 Addition

### ADR-004 addition (consumer-agnostic design)

> The query layer (`FilterSpec`, sort, grouping) is resource-specific by type but shares infrastructure. `FilterSpec` and `GroupSpec` are concrete types scoped to `PullRequest`. When a normalized `Issue` type is introduced, parallel `IssueFilterSpec` and `IssueGroupSpec` types follow the same structural pattern. The `Predicate[T any]` type and its `And`/`Or` combinators are shared across all resource types without modification. No cross-resource abstraction is introduced prematurely.

### ADR-006 addition (grouping and sort keys)

> **Resource scoping in config:** Named view definitions include a `resource` field (`"pr"` is the default and the only valid value in v1). Grouping and sort key validation is gated by the view's declared resource type. An invalid grouping or sort key for the declared resource type produces a startup error identifying the key and the valid alternatives. Silent fallback to `"none"` is not used.
>
> **Universal vs. resource-specific grouping keys:** The grouping keys `"none"`, `"repo"`, `"provider"`, and `"author"` are universal — valid for any resource type. The key `"review_status"` is PR-specific. Future resource types (Issues) define their own valid grouping extensions (`"milestone"`, `"assignee"`) under the same `by` field, validated against their declared resource type.
>
> **Sort keys follow the same principle:** `"updated"`, `"created"`, `"staleness"`, and `"title"` are universal. Resource-specific sort keys (`"ci_status"`, `"priority"`) are defined and validated per resource type when those resources are specced.
