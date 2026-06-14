# OpenTUI — Maintenance Continuity Research

Researched: 2026-06-13

---

## Project Origins & Motivation

OpenTUI (https://github.com/anomalyco/opentui) is a native terminal UI core written in Zig with TypeScript bindings. It was built by Anomaly Innovations (formerly the SST/Serverless Stack team), the same team behind OpenCode, terminal.shop, SST, OpenNext, and OpenAuth.

The project was built as an **internal tool first**, not a public framework. The genesis was the OpenCode AI coding agent, which originally used Go and Bubble Tea. As OpenCode scaled in complexity and user-facing polish requirements grew, the team found Bubble Tea's performance ceiling and architectural constraints limiting. OpenTUI replaced it in OpenCode v1.0 around 2025–2026. The team then open-sourced OpenTUI as a general TUI framework.

The stated motivation was performance and capability: Zig-powered rendering for correctness and speed, with TypeScript on top so application authors use familiar web-ecosystem tooling (React, SolidJS, or plain TS). The Zig core exposes a C ABI, making it bindable from any language. The README explicitly notes production use: "OpenTUI powers OpenCode in production today and will also power terminal.shop."

A Rust port (https://github.com/Dicklesworthstone/opentui_rust) has appeared independently, suggesting the core architecture is considered sound enough to be ported.

**Key fact about design intent:** The team built OpenTUI because they *needed* it. It is not a side project or experiment — it is the rendering layer for their flagship product (OpenCode, currently 150,000 GitHub stars, 6.5 million monthly active developers as of mid-2026) and will power their e-commerce product (terminal.shop). This creates a strong internal incentive to maintain it.

---

## Commit & Release Activity

- **Total releases:** 88 as of June 2026
- **Latest release:** v0.4.1, released June 11, 2026
- **Release cadence:** Approximately weekly; multiple releases per week is common. Sample from recent history:
  - v0.4.1 — June 11, 2026
  - v0.4.0 — June 9, 2026
  - v0.3.4 — June 7, 2026
  - v0.3.3 — June 7, 2026
  - v0.3.2 — June 4, 2026
  - v0.3.1 — June 1, 2026
  - v0.3.0 — May 28, 2026
  - v0.2.16 — May 26, 2026
  - v0.2.15 — May 20, 2026
  - v0.2.14 — May 18, 2026

This cadence reflects active, driven-by-production-needs development rather than release-and-stabilize patterns. The project is still pre-1.0 (roadmap phases: "Now" for v0.1–v0.5, "Next" for v0.x refactoring, "Later" for v1.0 migration of core systems to native).

**Language composition:** TypeScript 75.6%, Zig 20.1%, MDX/Astro/CSS remainder.

**Contributor count:** The contributor graph did not load in automated testing, but the project has 593 forks and an active community (see Community Adoption section). The codebase appears to be primarily driven by the Anomaly core team.

---

## Maintainer Health & Organizational Risk

**Who maintains it:** Anomaly Innovations (anomalyco on GitHub), headquartered in San Francisco, originally founded in Toronto. Core team: Jay V (CEO), Frank Wang (CTO), Dax Raad (co-founder), Adam Elmore.

**Funding history:**
- Founded 2019
- Y Combinator (2021) — $1.1M total raised
- SST (predecessor product) turned profitable in 2025
- No large VC round disclosed; the company appears deliberately small and cash-efficient

**Business model:** Open-source developer tools. Revenue comes from SEED (SST's managed deployment product) and likely commercial services around OpenCode. The company's philosophy is explicitly model-agnostic and open-source-first. Jay V has stated: "developers care deeply about tooling they can trust, modify, and truly own."

**Is OpenTUI core to Anomaly's business?** Yes. OpenTUI is the rendering engine for OpenCode (their flagship with 150,000 stars and 6.5M MAU) and terminal.shop (their e-commerce experiment). Abandoning OpenTUI would require rebuilding the rendering layer of their primary product. This is a strong structural incentive to maintain it.

**Organizational risk scenarios:**
- *Anomaly shuts down:* OpenTUI is MIT-licensed. The codebase would survive. However, 88 releases over ~12 months means the team has been the primary driver — community maintainership is unproven. A fork would likely occur given OpenCode's large user base, but momentum is unclear.
- *Anomaly pivots away from CLI:* Low probability given terminal.shop, OpenCode's success, and Jay V's stated conviction about terminal-native developer tooling. The team has been building terminal UIs since pre-OpenCode (terminal.shop generated $100K+ in first-year sales before OpenCode launched).
- *Key maintainer departure (Dax Raad is notable here as the TypeScript architect):* Unknown bus factor; the team is small (4–5 core people).

**Comparable precedent:** Charm (Bubble Tea maintainer) is a similarly lean VC-backed team. Both depend on open-source reputation for commercial success, creating alignment between community health and business survival.

---

## Community Adoption & Ecosystem

**GitHub metrics (as of June 13, 2026):**
- Stars: 11,800
- Forks: 593
- Topic repositories: 96 public repos tagged `#opentui`

**awesome-opentui** (https://github.com/msmps/awesome-opentui): 27 projects listed across 6 categories, 323 stars, 22 forks. Curated list actively maintained.

**Third-party projects (selected):**
- `tokscale` — CLI for tracking token usage across OpenCode/Claude Code sessions (3,700 stars)
- `critique` — Terminal UI for reviewing git changes (1,200 stars)
- `termcn` — Terminal UI component library, positioned as "shadcn for terminals" (515 stars)
- `t1code` — Terminal coding interface (488 stars)
- `waha-tui` — WhatsApp terminal UI (344 stars)
- `tuiboard` — Terminal kanban board (81 stars)
- `ghui` — TUI for managing open GitHub PRs across repositories (listed in awesome-opentui) — directly relevant to the prsm use case
- `hunk` — Review-first terminal diff viewer for agentic coders (in awesome-opentui)
- `critique` — Terminal interface for reviewing Git changes (in awesome-opentui)
- A CLI Solitaire game (56 stars)
- A DOOM-in-terminal implementation (42 stars)

**Observation on ecosystem signal:** The `#opentui` topic has 96 repos, which is meaningful given the project launched less than 18 months before this research date. Projects like `ghui` (a PR manager) indicate prsm would not be the first to attempt this space in OpenTUI — that is either a warning (competition) or a confirmation signal (validated category).

**What's missing:** No dedicated Discord server found. No community forum evident beyond GitHub Issues. Community concentration appears to be GitHub Issues + the awesome-opentui list. This is a mild concern for support quality relative to more established ecosystems.

---

## Zig Language Stability Risk

**Current Zig status:** Pre-1.0. Latest stable: 0.16 (released April 14, 2026). 1.0 is predicted for mid-to-late 2026, though no official date is set.

**Breaking change history per release (2024–2026):**
- 0.12 (early 2024): `std.os` → `std.posix` rename; `@fieldParentPtr` change; build system changes; `std.fmt` float formatting
- 0.13 (mid 2024): `std.Progress` rework; described as minor but breaking
- 0.14 (late 2024): `std.mem.split`/`tokenize` changes; multiple `std.builtin` changes; build system changes; labeled switch introduced
- 0.15 (2025): Major I/O overhaul ("Writergate") — all `std.io.Reader`/`Writer` deprecated in favor of new non-generic API. Andrew Kelley described this as "extremely breaking"
- 0.16 (April 2026): New `std.Io` interface; affects any code accessing system I/O

**Pattern:** Zig delivers one to two genuinely breaking API changes per release cycle. The pace will slow after 1.0 but there is no freeze yet.

**Who bears the Zig upgrade cost?** This is the critical question for a prsm team:

- OpenTUI's Zig layer is maintained by the Anomaly team. When Zig breaks, Anomaly upgrades OpenTUI.
- The TypeScript application author (e.g., a prsm developer) uses `@opentui/core` via npm. The README confirms Zig must be installed to *build* OpenTUI packages — but this appears to be a contributor/build requirement, not an end-user runtime requirement. Pre-built binaries are distributed via npm.
- The documentation states: install `bun add @opentui/core`, run `bun create tui`. This does not require the prsm developer to install or understand Zig.

**However:** If a prsm developer needs to modify OpenTUI, debug rendering issues, or stay on a specific Zig version, they will encounter Zig's churn. The abstraction leaks at the boundary.

**OpenTUI's stated response to Zig versions:** Not explicitly documented. Given 88 releases in ~12 months and the team's production dependency on the framework, it is reasonable to assume they track Zig releases. Issue #807 (markdown rendering failure in Bun-compiled binaries distributed via Homebrew — a path-sensitive bug in tree-sitter loading) shows that packaging edge cases exist that may be slow to resolve.

**Post-1.0 risk:** After Zig 1.0, breaking changes stabilize significantly. The main risk window is now through approximately end of 2026.

---

## TypeScript API Ecosystem (GitHub, GitLab, Gitea clients)

### GitHub — Octokit

**Package:** `octokit` / `@octokit/rest` (https://github.com/octokit/octokit.js)

- Maintained by GitHub itself (or a GitHub-blessed organization)
- Last updated: June 12, 2026 (multiple packages updated that date)
- Full TypeScript types across all packages
- Decomposable: use `@octokit/rest` for REST-only, `octokit` for batteries-included (REST + GraphQL + Apps)
- 100% test coverage claimed
- Created in 2012 (as `node-github`), continuously evolved
- Supports Browsers, Node.js, Deno, Bun

**Assessment:** Strongest option in this list. GitHub has a direct business interest in excellent JS/TS API clients. Extremely low abandonment risk.

### GitLab — Gitbeaker

**Package:** `@gitbeaker/rest` (https://github.com/jdalrymple/gitbeaker)

- Community-maintained (not official GitLab)
- Stars: 1,700
- Latest version: 43.8.0 (November 1, 2025)
- 347 total releases; 100+ contributors
- 98.8% TypeScript
- Supports Node.js, Deno, Bun, Browsers, CLI
- Covers GitLab API through v16.5
- Last release was November 2025 — 7 months before this research date, which is slightly stale but not alarming

**Alternative:** `@gitlab/application-sdk-browser` — the official GitLab SDK, but scoped to browser/analytics use cases, not general API access.

**Assessment:** Gitbeaker is the de facto standard. It is not officially maintained by GitLab, which means a long-term bus factor risk exists. The 347 releases show sustained effort. The Nov 2025 last release warrants monitoring but is not disqualifying.

### Gitea — gitea-js / @gitea/sdk

**gitea-js** (https://github.com/anbraten/gitea-js):
- Auto-generated from official Gitea OpenAPI definition
- Latest version: 1.23.0
- Last published: approximately 1 year before research date (mid-2025)
- Only 1 maintainer
- Only 7 other projects on npm using it
- Marked as having "not healthy version release cadence"

**@gitea/sdk** (official):
- Version 0.1.4 as of ~May 2026
- Very early; minimal track record
- MIT licensed

**Assessment:** The Gitea TypeScript ecosystem is the weakest of the three. `gitea-js` maps version numbers to Gitea API versions (e.g., 1.23.0 = Gitea 1.23 API), which is sensible architecture but the single-maintainer and low-activity status are genuine risks. For prsm, Gitea support would likely require either maintaining a fork of `gitea-js`, hand-rolling against the OpenAPI spec, or accepting the risk of the community package. The official `@gitea/sdk` at 0.1.4 is too early to depend on.

**Codeberg note:** Codeberg runs on Gitea, so the Gitea SDK covers Codeberg API calls.

---

## Single-Binary Distribution (Bun)

Bun (acquired by Anthropic in December 2025) provides `bun build --compile`, which produces a self-contained binary embedding the Bun runtime + application code. No Node.js installation required for end users.

**How it works:**
1. Write TypeScript entrypoint
2. Run `bun build --compile --target=<platform> ./src/cli.ts --outfile prsm`
3. Distribute the resulting binary (typically 50–80 MB for a full app)

**Practical example:** The Tigris CLI team used `bun build --compile` to ship a 60 MB self-contained binary for macOS, Linux, and Windows. The main challenge was replacing dynamic `import()` calls with static imports, handled via a code generation script.

**OpenTUI-specific complication:** There is an active unresolved bug (GitHub Issue #807, March 2026) where tree-sitter-based rendering (markdown, code, diff) silently breaks when an OpenTUI app is compiled with `bun build --compile` and installed to a path outside the build directory (e.g., via Homebrew). The issue is path-sensitive: tree-sitter's native libraries are not correctly bundled. This affects `MarkdownRenderable`, `CodeRenderable`, and `DiffRenderable`.

**Impact for prsm:** If prsm uses OpenTUI and targets Homebrew or other installer distribution (likely for a developer tool), this bug would affect any markdown or diff display in PR details. The bug was open as of the research date — timeline to fix is unknown.

**Workarounds noted:** Running from dev mode or from the build directory works. The failure is silent (no crash, just unformatted output), which may be worse from a UX perspective than an obvious error.

**Bun platform support:** macOS, Linux, Windows. Cross-compilation for all three targets is supported from a single macOS or Linux build machine.

**Runtime requirements for end user:** None — the compiled binary is fully self-contained. Bun 1.3 (October 2025) is the latest stable; the Anthropic acquisition positions Bun for continued investment, particularly around CLI tooling.

---

## Summary Assessment

OpenTUI is maintained by a lean but credible team (Anomaly Innovations) with genuine production dependency on the framework — it powers OpenCode (150,000 GitHub stars, 6.5M MAU) and will power terminal.shop. This is the strongest continuity signal available: the maintainers cannot afford to abandon it without rebuilding their own products. The 88 releases in roughly 12 months, weekly release cadence, and 11,800 stars demonstrate active development and meaningful community traction. The 96 `#opentui` topic repositories — including at least one direct PR-management TUI (`ghui`) — show the ecosystem is already being used for the exact category prsm targets.

The risks are real but bounded. Zig's pre-1.0 status means the team must absorb one to two breaking upgrades per year until approximately late 2026; the TypeScript application author is insulated from this as long as Anomaly keeps pace (so far they have). The Gitea TypeScript SDK ecosystem is immature (single-maintainer, stale), which creates maintenance burden for Gitea/Codeberg support regardless of TUI framework choice. The `bun build --compile` distribution bug affecting tree-sitter rendering (Issue #807) is a concrete unresolved problem for Homebrew/binary distribution today.

**Overall confidence rating: Medium.** OpenTUI is well-motivated and actively maintained by a team with structural incentive to keep it alive. It is not a risk-free choice — it is pre-1.0, small-team, and carries Zig version churn — but it is meaningfully safer than a typical early-stage open-source project because it is load-bearing infrastructure for its own creators. The primary hedge is the MIT license: if Anomaly were to stop maintaining it, a fork would be viable given the active community. The decision hinges on whether prsm can accept the TypeScript-ecosystem tradeoffs (weaker Gitea client, one open binary distribution bug) versus the Go ecosystem where Bubble Tea is more mature and the Gitea Go SDK is healthier.

---

## Sources

- OpenTUI GitHub repository: https://github.com/anomalyco/opentui
- OpenTUI website: https://opentui.com
- OpenTUI roadmap issue #821: https://github.com/anomalyco/opentui/issues/821
- OpenTUI binary distribution bug #807: https://github.com/anomalyco/opentui/issues/807
- Awesome OpenTUI list: https://github.com/msmps/awesome-opentui
- OpenTUI GitHub topic: https://github.com/topics/opentui
- OpenCode background story: https://techfundingnews.com/opencode-the-background-story-on-the-most-popular-open-source-coding-agent-in-the-world/
- Anomaly Innovations (Crunchbase): https://www.crunchbase.com/organization/anomaly-innovations
- Zig 0.16 release notes: https://ziglang.org/download/0.16.0/release-notes.html
- Zig 1.0 timeline (Medium): https://techpreneurr.medium.com/zig-1-0-drops-in-2026-why-c-developers-are-secretly-learning-it-now-3188f8bcfedf
- Zig 0.15 "extremely breaking" std.io: https://devclass.com/2025/07/07/zig-lead-makes-extremely-breaking-change-to-std-io-ahead-of-async-and-awaits-return/
- Zig 0.15 migration roadblocks: https://sngeth.com/zig/systems-programming/breaking-changes/2025/10/24/zig-0-15-migration-roadblocks/
- Octokit GitHub: https://github.com/octokit/octokit.js
- Octokit npm: https://www.npmjs.com/package/octokit
- Gitbeaker (GitLab SDK): https://github.com/jdalrymple/gitbeaker
- gitea-js: https://github.com/anbraten/gitea-js
- gitea-js npm: https://www.npmjs.com/package/gitea-js
- @gitea/sdk npm keywords search: https://www.npmjs.com/search?q=keywords:gitea
- Bun single-file executables: https://bun.com/docs/bundler/executables
- Bun Tigris CLI case study: https://www.tigrisdata.com/blog/using-bun-and-benchmark/
- OpenTUI Rust port: https://github.com/Dicklesworthstone/opentui_rust
- OpenCode at 150k stars (AI Wiki): https://aiwiki.ai/wiki/opencode
- Zig type resolution redesign 2026: https://sesamedisk.com/zig-type-resolution-redesign-2026/
- Zig 0.16 new features: https://daily.dev/blog/zig-0-16-new-features-release-date-developers-need-to-know/
