# Multi-Resource View Field: Analysis and Recommendation

## What Is Hard to Change

The config schema is a public API. Once users have `~/.config/prsm/config.toml` with named views, any rename or structural change to those keys is a breaking change requiring a migration story. Specifically:

- **The field name** (`resource`, `type`, `kind`) — renaming it later requires either a deprecation cycle with two supported names or a `prsm config migrate` command.
- **The value vocabulary** (`"pr"`, `"issue"`, `"git.pr"`) — if future resource types reveal the vocabulary was too narrow (e.g., Jira issues are not "git" objects), correcting the namespace requires changing values users have already written, not just adding new ones.
- **Whether it is optional or required** — if the field starts optional (defaulting to `"pr"`), making it required later breaks every existing config that omitted it.

What is less hard to change: the *implementation* that dispatches on the field value, the TUI behavior, the set of filter keys valid per resource type. Those live in Go code, not user files.

## Recommendation

Use `resource = "pr"` as a required string field on every `[[views]]` entry, with the following vocabulary from the start: `"pr"`, `"issue"`. Do not namespace it (not `"git.pr"`).

Rationale:

**`resource` is the right key name.** `type` is already used on `[[providers]]` (e.g., `type = "github"`) — reusing `type` on views creates conceptual ambiguity between "provider type" and "view resource type." `kind` has no established meaning in this config and is less self-documenting. `resource` is unambiguous: it names what the view is a view *of*.

**Flat values, not namespaced.** `"git.pr"` anticipates a non-git issue tracker (Jira, Linear) that is explicitly out of v1 scope and remains speculative. Namespacing prematurely creates two problems: it makes every v1 view definition more verbose for no immediate benefit, and if the namespace taxonomy turns out wrong (maybe Jira issues are `"jira.issue"` or `"tracker.issue"`), the correction is a breaking change. Keep it `"pr"` and `"issue"`. When/if a non-git resource is added, its value is determined by what it actually is — the vocabulary can grow without invalidating existing values.

**Make it required, but default to `"pr"` for backward compatibility.** When prsm encounters a `[[views]]` entry with no `resource` field, it defaults to `"pr"` and logs a deprecation notice at startup: `view "my-reviews": resource not set; defaulting to "pr". Add resource = "pr" to silence this warning.` This makes the field effectively required going forward without breaking existing configs. It is the same pattern used by many Go CLIs to evolve a field from implicit to explicit without a hard break.

**Views are strictly typed to one resource.** A view shows either PRs or issues, never both interleaved. Mixed-resource views are superficially appealing ("see everything assigned to me") but create three concrete problems: (1) the filter key space diverges between resource types — a `reviewer` filter is PR-specific; `assignee` is issue-specific; a mixed view has no coherent schema for `[views.filter]`; (2) sort and group semantics differ (`review_status` grouping is meaningless for issues); (3) in the TUI, mixed rows require heterogeneous rendering in a single list, which is substantially more complex in Bubble Tea. The right answer for cross-resource triage is a dedicated dashboard view (a future feature), not mixed-type `[[views]]`.

**The view picker shows all views, filtered by context.** When the user is in PR mode, the view picker shows PR views first but surfaces issue views as a separate section (or dims them). This is simpler to implement than hiding them entirely, and it lets users switch resource context deliberately via the view picker — which may be the natural "mode switch" UX anyway.

## What Is NOT Hard to Change

- Adding new resource values (`"issue"`, future values) — additive, no existing config breaks.
- Adding issue-specific filter keys to `FilterSpec` and the corresponding TOML validation — no changes to existing `[[views]]` entries.
- The TUI panel layout when switching between PR and issue resource types — implementation detail.
- Whether the view picker dims vs. hides views of the inactive resource type — UX detail, changeable without config impact.
- The internal `FilterSpec` struct gaining resource-typed variants (e.g., `IssueFilterSpec`) — internal Go types, not config schema.

## Suggested ADR-005 / ADR-006 Addition

Add to **ADR-005**, in the "View definition schema" section, after the intro paragraph:

> **Resource type.** Every view targets exactly one resource type, specified by the `resource` field. Supported values in v1: `"pr"` (pull requests). The value `"issue"` is reserved for a future release. Views with no `resource` field default to `"pr"` with a deprecation warning at startup; new configs must include the field explicitly.

Add to **ADR-005**, in the annotated example config, on each `[[views]]` block:

```toml
[[views]]
name        = "my-reviews"
description = "PRs where I am a requested reviewer, non-draft only"
resource    = "pr"
```

Add to **ADR-006**, in the "Named views and session filter interaction" section:

> **Views are resource-typed.** A named view's filter, sort, and grouping fields are evaluated against a single resource type (as declared by `resource` in `[[views]]`). A view with `resource = "pr"` can only express filters defined in the PR `FilterSpec`; issue-specific fields (e.g., `assignee`) are invalid in a PR view and produce a startup validation error. When multi-resource support ships, the view picker surfaces all views but groups them by resource type, allowing deliberate context switching.
