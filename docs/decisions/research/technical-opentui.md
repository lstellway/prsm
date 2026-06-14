# OpenTUI — Technical Strengths Research

**Date:** 2026-06-13
**Status:** Raw research for synthesis
**Purpose:** Fair technical evaluation of OpenTUI for prsm, correcting a prior dismissal based on an incorrect assumption about TypeScript API client coverage.

---

## Architecture & Rendering Model

### Overview

OpenTUI is a TUI framework built on a dual-language architecture: a native Zig core (75% TypeScript, 20% Zig by line count in the repo) that exposes a C ABI, plus TypeScript bindings that application authors write against. The Zig layer handles all terminal-critical operations; TypeScript is the authoring surface.

**Repository:** https://github.com/anomalyco/opentui  
**Docs:** https://opentui.com  
**npm:** `@opentui/core`  
**Stars:** ~11k as of mid-2026

### Rendering Pipeline (8 Stages)

The rendering pipeline runs in a fixed sequence each frame:

1. **Request** — `requestRender()` marks dirty components and debounces
2. **Loop** — FPS-capped execution (default 30 FPS, up to 60+)
3. **Layout** — Yoga flexbox engine computes positions/sizes
4. **Render** — Components draw to `OptimizedBuffer` via `renderable.render()`
5. **Hit Grid** — Zig FFI maps screen coordinates to renderable IDs for mouse events
6. **Diff** — Native frame diffing; cell arrays compared, changed regions identified
7. **ANSI** — Run-length encoding collapses adjacent identical-style cells
8. **Output/Swap** — Buffered write to terminal; buffer references exchanged

The shadow-buffer (double-buffered) model means only changed cells produce ANSI output. For a two-panel layout where the PR list refreshes and the detail panel is static, only the list column emits escape sequences.

### How the Zig FFI Bridge Works

`Bun.dlopen()` loads the correct platform binary at startup. The module `zig-renderlib.ts` selects from pre-built optional npm packages:

- `@opentui/darwin-arm64`
- `@opentui/darwin-x64`
- `@opentui/linux-x64`
- `@opentui/linux-arm64`
- `@opentui/win32-x64`
- `@opentui/win32-arm64`

Bun's FFI claims "near-zero overhead" on cross-language calls, enabling sub-millisecond frame times without the overhead of typical IPC. Text buffers (rope-based), rendering, and hit detection run in the Zig layer.

### Comparison to Bubble Tea and Ratatui

| Dimension | OpenTUI | Bubble Tea v2 (Go) | Ratatui (Rust) |
|---|---|---|---|
| Architecture | Retained-mode reconciler (React/SolidJS) + native Zig diff | Elm/MVU retained-mode | Immediate-mode (redraw every frame) |
| Frame diffing | Zig native, per-cell shadow buffer | ncurses-based "Cursed Renderer" (v2, Feb 2026) | Rust double buffer |
| Stars | ~11k | ~40k | ~19k |
| Stability | Pre-release (v0.1.x); explicit "not ready for production use" | Stable v2 (Feb 2026) | Stable v0.30.x |
| Paradigm | React/SolidJS JSX | Elm-style update functions | Closure-based immediate mode |
| FPS claim | 60+ | "orders of magnitude" faster than v1 | "sub-millisecond" |
| Mode 2026 (DEC sync) | Not confirmed | Yes | Not confirmed |

**Why SST moved from Bubble Tea to OpenTUI:** The team originally wrote OpenCode in Go with Bubble Tea, then rewrote in TypeScript. Rather than accept Ink's known ceiling (hardcoded 30 FPS, >50 MB memory for simple apps), they sponsored the OpenTUI author (kmdrfx) to build a high-performance TypeScript-native alternative. The goal was TypeScript developer ergonomics combined with native rendering speed. Source: https://aiwiki.ai/wiki/opencode

---

## Multi-Panel Layout Ergonomics

### Layout Engine

OpenTUI uses Facebook's **Yoga** layout engine (the same engine used in React Native), which implements a substantial CSS Flexbox subset. Layout computations happen in Zig for performance, but developers interact with it through TypeScript.

### Supported Flexbox Properties

- **`flexDirection`**: `"row"`, `"column"`, `"row-reverse"`, `"column-reverse"`
- **`justifyContent`**: `"flex-start"`, `"flex-end"`, `"center"`, `"space-between"`, `"space-around"`, `"space-evenly"`
- **`alignItems`**: `"flex-start"`, `"flex-end"`, `"center"`, `"stretch"`, `"baseline"`
- **`flexGrow`**, **`flexShrink`**, **`flexBasis`**: for proportional sizing
- **`width`/`height`**: fixed integers (character cells) or `"100%"` percentages
- **`padding`**, **`paddingX`**, **`paddingY`**, **`paddingTop/Right/Bottom/Left`**
- **`gap`**: spacing between children
- **`position`**: `"relative"` (default) or `"absolute"` with `left/top/right/bottom`

### Two-Panel Layout (PR List + Detail) — Official Example

The homepage explicitly shows a sidebar + main layout, which maps directly to prsm's list+detail requirement:

```typescript
import { createCliRenderer, Box, Text } from "@opentui/core"

const renderer = await createCliRenderer()

renderer.root.add(
  Box(
    { width: "100%", height: "100%", flexDirection: "row", gap: 2 },
    Box(
      { flexGrow: 1, backgroundColor: "#1a1b26" },       // PR list pane
      Text({ content: "PR List", fg: "#bb9af7" })
    ),
    Box(
      { flexGrow: 3, backgroundColor: "#24283b" },       // PR detail pane
      Text({ content: "PR Detail" })
    )
  )
)
```

Alternatively, with the imperative API:

```typescript
const container = new GroupRenderable(renderer, {
  flexDirection: "row",
  width: "100%",
  height: "100%",
})
const listPane = new BoxRenderable(renderer, {
  width: 30,                // Fixed character-column width
  backgroundColor: "#333",
})
const detailPane = new BoxRenderable(renderer, {
  flexGrow: 1,              // Fill remaining space
  backgroundColor: "#111",
})
container.add(listPane)
container.add(detailPane)
```

### Scrollable Containers

The `ScrollBox` component handles scrollable content. A PR list can be wrapped in `ScrollBox` with keyboard navigation wired to scroll position. `TextBufferRenderable` and `ScrollBoxRenderable` are first-class built-in components.

### Responsive Resize

OpenTUI exposes a `"resize"` event that can dynamically update flex directions. This handles terminal window resizing, which is important for the list+detail layout collapsing on narrow terminals.

### Built-in Components

16+ built-in components including: `Input`, `Textarea`, `Select`, `TabSelect`, `ScrollBox`, `Slider`, `Code` (with tree-sitter syntax highlighting), `Markdown`, `Diff`, `QR Code`, `FrameBuffer`, `ASCIIFont`.

Source: https://opentui.com/docs/core-concepts/layout/

---

## Concurrent/Live Update Story

### The Node.js/Bun Event Loop Model

Since OpenTUI TypeScript code runs on Bun (or Node.js 26.3+), it inherits the event loop model natively. There is no additional "message passing" layer the way Go channels or Rust's tokio channels work in Bubble Tea/Ratatui. Background API polling fits naturally into standard async JavaScript patterns.

### Pattern 1: React `useEffect` + `setInterval`

With `@opentui/react`:

```typescript
function PRList() {
  const [prs, setPRs] = useState<PR[]>([])

  useEffect(() => {
    const fetchPRs = async () => {
      const data = await octokit.rest.pulls.list({ owner, repo })
      setPRs(data.data)
    }

    fetchPRs() // initial fetch
    const id = setInterval(fetchPRs, 60_000) // poll every 60s
    return () => clearInterval(id) // cleanup on unmount
  }, [])

  return <ScrollBox height="100%">{prs.map(pr => <PRRow pr={pr} />)}</ScrollBox>
}
```

This is idiomatic React and works identically in OpenTUI's React reconciler. The `requestRender()` debounce batches rapid state changes so a burst of PR updates does not produce 50 individual frames.

### Pattern 2: SolidJS `createResource` + `refetch`

With `@opentui/solid` (the approach used in OpenCode itself):

```typescript
const [prs, { refetch }] = createResource(fetchPRs)

// Trigger background polling
setInterval(refetch, 60_000)

// SolidJS fine-grained reactivity auto-updates only affected nodes
return <ScrollBox>{() => prs()?.map(pr => <PRRow pr={pr} />)}</ScrollBox>
```

SolidJS `createResource` has built-in `loading` and `error` reactive properties. The fine-grained reactivity model means only DOM nodes that read a changed signal re-render — not the whole component tree.

### Pattern 3: Concurrent multi-provider polling

For prsm's multi-provider requirement (GitHub + GitLab + Gitea polling independently):

```typescript
// Each provider polls independently; updates merge into shared signal
async function pollProvider(provider: Provider, interval: number) {
  while (true) {
    try {
      const prs = await provider.fetchPRs()
      allPRsSignal.update(prev => mergeByProvider(prev, provider.id, prs))
    } catch (e) {
      providerErrorSignal.set(provider.id, e)
    }
    await sleep(interval)
  }
}

// Start all provider pollers
providers.forEach(p => pollProvider(p, 60_000))
```

The single-threaded event loop interleaves these naturally. Bun's `Bun.sleep()` and `fetch()` are both non-blocking. No explicit concurrency management needed beyond standard async/await.

### OpenCode's Production Pattern

OpenCode uses SSE (Server-Sent Events) rather than polling, via a `SyncProvider` component that subscribes through the SDK client. For prsm's HTTP polling use case, the event loop + `setInterval` pattern is simpler and sufficient. The key point: OpenTUI imposes no constraints on async patterns; it is just the rendering layer.

---

## Vim Keybinding Support

### How Keybindings Work

Input flows through parsing layers supporting:
- Standard ANSI sequences (default, universal compatibility)
- `modifyOtherKeys` protocol (`otherModifiersMode: true`)
- Kitty Keyboard protocol (`kittyKeyboard: true`)

The `key-handler.ts` module routes events to focused components. OpenCode uses a `registerOpencodeKeymap` function in `keymap.tsx` that maps named commands to physical key events, read from a user-configurable `tui.json`. The architecture supports global keybindings (always active) and scoped/contextual keybindings (active only in a specific view).

### Vim Keybindings in Practice

Vim keybindings are **not built-in out of the box** — you wire them yourself using the keymap system. A community developer added vim keybindings to OpenCode's TUI (post-v1.0) by implementing a mode-switching layer: `createScopedKeymap` was referenced in the OpenCode codebase as the scoping primitive. The approach was:

1. Track `mode` state (`"normal"` | `"insert"`)
2. In normal mode, intercept `h/j/k/l` and map to scroll commands
3. In insert mode, pass keys through to the `TextareaRenderable`
4. Register `/` to focus a filter input

Source: https://sngeth.com/opencode/vim/typescript/solidjs/tui/2026/03/11/adding-vim-keybindings-to-opencode/ (403 on direct fetch, but referenced in GitHub issues)

### Known Keybinding Issues

- GitHub Issue #6016 (sst/opencode, Dec 2025): Global keybinds execute in background when dialogs/pickers are open — the scoped keymap system had a leakage bug with modal dialogs.
- GitHub Issue #3690 (Nov 2025): After the OpenTUI v1.0 migration, some vim-like keybinds were silently removed.
- Both indicate the keybinding system works but had rough edges around scoping during the 0.x→1.0 transition period.

### Assessment for prsm

Vim keybindings for prsm (`hjkl` navigation, `/` to filter, `?` for help) are implementable but require manual wiring. There is no built-in "vim mode" library for OpenTUI comparable to Bubble Tea's `key.Matches()` helper or the well-known `tui-textarea` crate pattern in Ratatui. The building blocks exist (key event parsing, focus management, scroll boxes), but the modal keybinding system is user-implemented.

---

## TypeScript API Ecosystem (GitHub, GitLab, Gitea clients)

**The prior research dismissal of OpenTUI on ecosystem grounds was incorrect.** The TypeScript/JavaScript ecosystem has mature, actively maintained API clients for all three target providers.

### GitHub: Octokit

**Package:** `@octokit/rest`, `@octokit/graphql`, `octokit` (batteries-included)  
**Repo:** https://github.com/octokit/octokit.js  
**Maturity:** Highest available. Originally created in 2012 as `node-github`. 100% test coverage, extensive TypeScript declarations, 100% REST API coverage, GraphQL support.

**PR-relevant APIs:**
- `octokit.rest.pulls.list()` — list PRs with filter params
- `octokit.rest.pulls.listReviews()` — review status
- `octokit.rest.checks.listForRef()` — CI status
- `octokit.graphql()` — full GraphQL for complex queries (check runs, review threads, etc.)

**Authentication:** OAuth tokens, GitHub Apps, PATs — all first-class.

**Assessment:** No gaps for prsm's needs. This is the best-supported provider client in any language.

### GitLab: Gitbeaker

**Package:** `@gitbeaker/rest`  
**Repo:** https://github.com/jdalrymple/gitbeaker  
**Stars:** 1.7k  
**Last release:** v43.8.0 (November 1, 2025) — includes reviewers sub-resource for MergeRequests  
**Maintenance:** Active, 347 releases, CircleCI CI passing

**Coverage:** All GitLab REST APIs up to GitLab 16.5. Merge requests, reviews, labels, pipelines, CI status. Works on Node.js, Deno, Bun, and browsers.

**TypeScript:** 98.8% TypeScript, extensive type declarations. Listed on GitLab's official third-party clients page.

**Important note:** There is no official first-party GitLab JavaScript/TypeScript SDK. Gitbeaker is the de facto standard.

**Assessment:** Strong coverage. Minor concern: community-maintained rather than GitLab-official, but actively developed and GitLab-listed.

### Gitea: gitea-js

**Package:** `gitea-js` (primary option)  
**Repo:** https://github.com/anbraten/gitea-js  
**Stars:** 51  
**Maturity:** Auto-generated from Gitea's official OpenAPI definition. Version-mapped (library version tracks Gitea API version). v1.23.0 released January 13, 2025. MIT license.

**Coverage:** Complete — all endpoints from the OpenAPI spec including pull requests.

```typescript
const api = giteaApi('https://try.gitea.com/', { token: 'access-token' })
const prs = api.repos.repoListPullRequests('owner', 'repo')
```

**Limitations:** Low stars (51), small community, requires `cross-fetch` polyfill for Node.js.

**Alternative:** An official Gitea JS/TS SDK from `techknowlogick` exists at v0.1.4 as of 2025.

**Assessment:** Functionally adequate but low maturity signal. The auto-generation from OpenAPI is a strength (spec-complete, consistent), but the project is lightly maintained.

### Comparison: TypeScript vs. Go API Client Ecosystem

| Provider | TypeScript Client | Go Client |
|---|---|---|
| GitHub | `@octokit/rest` — 2012 origin, 100% coverage, 100% tests, official | `google/go-github` — ~9.8k stars, actively maintained |
| GitLab | `@gitbeaker/rest` — 1.7k stars, community, GitLab-listed | `xanzy/go-gitlab` — well-established community client |
| Gitea | `gitea-js` — 51 stars, auto-generated from OpenAPI | `gitea.dev/sdk` — official Gitea Go SDK |

**Conclusion:** The TypeScript ecosystem is weaker on GitLab (no official SDK) and Gitea (very low stars), but functionally adequate for all three. The Go clients for GitLab and Gitea are also community-maintained and similarly low in stars for the smaller providers. The prior dismissal of OpenTUI's TypeScript ecosystem was not justified — for Octokit alone, the TypeScript client is arguably the best-maintained GitHub API client in any language.

---

## Distribution Model (Bun single-binary)

### Compilation

```bash
bun build ./src/index.ts --compile --outfile prsm
```

Produces a standalone binary with no runtime dependency — Bun is embedded in the executable. Cross-compilation is supported:

```bash
bun build ./src/index.ts --compile --target=bun-linux-x64 --outfile prsm-linux-amd64
```

Supported targets: Linux x64/arm64 (including musl variants), Windows x64/arm64, macOS x64/arm64.

### Binary Size

A "hello world" `bun build --compile` program produces approximately **57 MB** on `darwin-arm64`. A real application will be larger. The Bun runtime is embedded in every binary — this is the primary size driver, not application code.

Go binaries by comparison are typically 5–15 MB for a comparable CLI tool. Rust binaries with Ratatui are 2–8 MB.

The `--minify` and `--bytecode` flags reduce size somewhat and improve startup time ("~2x faster startup" per Bun docs).

### Critical Distribution Limitation: TreeSitter Workers

OpenTUI's `MarkdownRenderable`, `CodeRenderable`, and `DiffRenderable` use `TreeSitterClient`, which spawns a Web Worker. When compiled with `bun build --compile`, the worker entry point (`parser.worker.js`) is **not automatically bundled** (GitHub Issue #807, still unresolved as of early 2026).

**Effect on prsm:** prsm would likely use `DiffRenderable` for showing PR diffs and possibly `MarkdownRenderable` for PR descriptions. Both would silently degrade to plain text in distributed Bun binaries.

**Workaround documented:** Users must manually ship `parser.worker.js` alongside the binary and set `OTUI_TREE_SITTER_WORKER_PATH`. This is a non-starter for a clean single-binary distribution.

**Alternative:** OpenTUI could use `Code` component without the tree-sitter markdown renderable, and implement diff display using custom `DiffRenderable` that avoids the worker problem. Not confirmed as viable without testing.

### Native Zig Binaries

The pre-built Zig shared libraries are distributed as separate optional npm packages (`@opentui/darwin-arm64`, etc.). When using `bun build --compile`, these need to be properly bundled. The FFI loading via `Bun.dlopen()` may have interaction issues with the compiled binary's virtual filesystem (`/$bunfs/root/`). This is documented but the exact bundling behavior for Bun's virtual FS with FFI binaries requires verification.

### Node.js Runtime

Native rendering requires:
- Bun (recommended), OR
- Node.js 26.3.0 with `--experimental-ffi` flag

Node.js 26.x is the "Current" release line as of 2026. The `--experimental-ffi` flag is an ongoing concern for deployment environments expecting stable Node.js APIs.

NAPI support (Issue #2) was closed with an associated PR (#1149), suggesting work was done to address broader runtime compatibility, but the status is unclear from public information.

---

## Developer Ergonomics

### Learning Curve from Go/Rust Background

**Favorable factors:**
- React/SolidJS component model is well-documented and widely understood
- Flexbox layout is familiar to anyone with web development experience
- TypeScript has excellent tooling (LSP, type checking, IDE support)
- `bun create tui` scaffolds a project in seconds
- The `@opentui/core` imperative API is simple to understand

**Friction factors:**
- Requires installing **both** Bun and Zig (for building from source or modifying Zig code)
- TypeScript changes test immediately; Zig changes require `bun run build:native`
- Terminal-specific mental model: character grids, not pixels — requires learning ANSI, character widths, full/half-width Unicode
- No built-in vim modal system — must implement from primitives
- Limited community component ecosystem (no equivalent of Bubble Tea's Bubbles library)
- `@opentui/vue` is marked unmaintained; only React and SolidJS are actively supported
- Local development requires symlink management for monorepo linking

### Minimal Two-Panel "Hello World" Boilerplate

```typescript
import { createCliRenderer, Box, Text, ScrollBox } from "@opentui/core"

const renderer = await createCliRenderer({ exitOnCtrlC: true })

const layout = Box(
  { width: "100%", height: "100%", flexDirection: "row" },
  Box(
    { width: 30, borderStyle: "rounded", flexDirection: "column" },
    Text({ content: "PR List", fg: "#bb9af7" }),
    // items added dynamically
  ),
  Box(
    { flexGrow: 1, borderStyle: "rounded", padding: 1 },
    Text({ content: "Select a PR", fg: "#565f89" }),
  )
)

renderer.root.add(layout)

renderer.on("keydown", (event) => {
  if (event.key === "j") { /* move down */ }
  if (event.key === "k") { /* move up */ }
  if (event.key === "q") process.exit(0)
})
```

This is concise for a Go developer. The main cognitive shift is from Go's explicit channel/goroutine model for key handling to event listener callbacks.

### Pain Points Reported by Developers

1. **Pre-production warning:** `@opentui/core` is explicitly "not ready for production use" (v0.1.x). API may change without semver guarantees.
2. **Keybinding scoping bugs:** Global keys leaking into modal dialogs (Issue #6016).
3. **TreeSitter binary distribution bug:** Silent failure in compiled binaries (Issue #807, unresolved).
4. **Zig toolchain requirement:** Developers contributing Zig code must install and understand Zig (pre-1.0, syntax still changing).
5. **Sparse third-party component libraries:** No mature widget ecosystem comparable to Bubble Tea's Bubbles.
6. **Vue unmaintained:** Only React and SolidJS are live targets.

### Zig Pre-1.0 Status Impact on TypeScript Authors

Zig 0.16 (Beta) was released April 14, 2026. Zig is still pre-1.0 and makes breaking syntax/semantic changes between versions. **This only affects TypeScript authors if they need to modify or contribute to OpenTUI's Zig core.** If you are only using the TypeScript API, Zig's pre-1.0 status is invisible — you consume pre-built binaries downloaded as npm optional dependencies. The risk is that upstream OpenTUI Zig code breaks with a new Zig version and maintainers don't keep up, but this is an upstream maintenance risk, not a day-to-day author concern.

---

## Performance Characteristics

### Claims

- Sub-millisecond frame times (frame diffing runs in Zig native code)
- 60+ FPS rendering for complex UIs
- Shadow buffer diff only writes changed cells to terminal
- ANSI RLE encoding minimizes escape sequence output

### Evidence

- **Production validation:** OpenCode (AI coding agent in active production use) is the primary proof. SST explicitly abandoned Bubble Tea because they needed higher performance for AI streaming output. The fact that OpenCode shipped v1.0 on OpenTUI is the strongest available signal.
- **No public benchmarks:** No published benchmark comparing OpenTUI vs. Bubble Tea vs. Ratatui frame rates on comparable workloads. Performance claims are qualitative.
- **Ink comparison:** OpenTUI claims to be significantly better than Ink (React-based TUI for Node.js) which has a hardcoded 30 FPS cap and >50 MB memory footprint for simple apps. This comparison is plausible given the Zig native rendering path.

### Bubble Tea v2 Context (Feb 2026)

Bubble Tea v2 released February 2026 with a "Cursed Renderer" described as "orders of magnitude" faster than v1, plus DEC synchronized output (atomic frame updates). This closes much of the raw performance gap that motivated OpenTUI's creation. However, Bubble Tea v2 remains Go-only, and no head-to-head benchmark against OpenTUI has been published.

### For prsm's Workload

prsm's performance requirements are modest compared to an AI coding agent streaming thousands of tokens/second. A 60-second polling cycle with a list of ~200 PRs and a detail panel update is well within what either Bubble Tea v2 or OpenTUI can handle. Raw frame performance is not the discriminating factor for prsm.

---

## Summary Assessment

OpenTUI is a technically interesting framework that delivers on its core promise: React/SolidJS developer ergonomics with native Zig rendering performance. For prsm, the picture is mixed.

**Strengths that fit prsm well:**
- Two-panel layout with Yoga flexbox is straightforward and well-documented — the exact list+detail pattern is shown in official examples
- Async API polling works naturally via the Node.js/Bun event loop with `useEffect` + `setInterval` or SolidJS `createResource` — no special OpenTUI primitives needed
- The TypeScript API client ecosystem (Octokit for GitHub, Gitbeaker for GitLab, gitea-js for Gitea) is functionally adequate for prsm's needs; the prior dismissal of TypeScript ecosystem coverage was incorrect
- Built-in `ScrollBox`, `Code`, `Diff`, and `Markdown` components cover prsm's display needs

**Concerns that create real risk for prsm:**
- **Distribution is the hardest problem.** The TreeSitter worker bundling bug (Issue #807, unresolved) would silently break `DiffRenderable` and `MarkdownRenderable` in compiled Bun binaries. For a tool targeting developers who install via Homebrew or a pre-built binary, this is a critical gap.
- **~57 MB minimum binary size** is roughly 4–10x larger than a comparable Go binary. Not a dealbreaker but materially worse for download/install UX.
- **Explicit "not production ready" warning** on `@opentui/core` (v0.1.x). API stability is not guaranteed. For a project where prsm's users would depend on a specific version, this is a risk.
- **Vim keybinding system requires manual implementation.** The building blocks exist but a modal keymap with scoping (normal/insert mode, `/` search, `?` help) requires non-trivial wiring, and known bugs around modal dialog scoping (Issue #6016) suggest the foundation had rough edges through late 2025.
- **Bun as a runtime requirement** is a non-standard dependency for Go/Rust developers who are prsm's likely early adopters; `bun create tui` is less familiar than `go build`.

**Net assessment:** OpenTUI is a credible choice for TypeScript-first teams building developer tools and willing to operate on the cutting edge. The TypeScript ecosystem coverage is not the dealbreaker it was claimed to be — Octokit is excellent, and Gitbeaker is functional. The real blockers for prsm are the Bun compiled-binary distribution limitations (specifically the TreeSitter worker bug for diff/markdown rendering) and the explicit pre-production status of the library. If the TreeSitter bundling bug were fixed and the library reached v1.0, OpenTUI would be a strong contender. As of mid-2026, Bubble Tea v2 (Go) is the lower-risk choice for prsm's distribution model, while OpenTUI remains compelling for teams already invested in the TypeScript ecosystem who can accept the current rough edges.

---

## Sources

- OpenTUI GitHub: https://github.com/anomalyco/opentui
- OpenTUI Docs: https://opentui.com/docs/getting-started/
- OpenTUI Layout Docs: https://opentui.com/docs/core-concepts/layout/
- OpenTUI npm: https://www.npmjs.com/package/@opentui/core
- DeepWiki (sst/opentui): https://deepwiki.com/sst/opentui
- DeepWiki (OpenCode TUI): https://deepwiki.com/sst/opencode/6.2-terminal-user-interface-(tui)
- DeepWiki (OpenCode Keybindings): https://deepwiki.com/anomalyco/opencode/9.2-tui-commands-and-keybindings
- OpenCode AI Wiki: https://aiwiki.ai/wiki/opencode
- Stork.AI OpenTUI Article: https://www.stork.ai/blog/the-tui-library-thats-killing-ink
- Starlog OpenTUI Architecture: https://starlog.is/articles/developer-tools/anomalyco-opentui/
- Bun Single-File Executables: https://bun.com/docs/bundler/executables
- Bun FFI: https://bun.com/docs/runtime/ffi
- Issue #807 (TreeSitter binary bug): https://github.com/anomalyco/opentui/issues/807
- Issue #2 (NAPI support): https://github.com/anomalyco/opentui/issues/2
- Issue #6016 (keybind modal leak): https://github.com/sst/opencode/issues/6016
- Octokit.js: https://github.com/octokit/octokit.js/
- Gitbeaker: https://github.com/jdalrymple/gitbeaker
- gitea-js: https://github.com/anbraten/gitea-js
- Awesome OpenTUI: https://github.com/msmps/awesome-opentui
- ghui (GitHub PR TUI built on OpenTUI): https://github.com/kitlangton/ghui
- TUI Comparison (melker/agent_docs): https://github.com/wistrand/melker/blob/main/agent_docs/tui-comparison.md
- Bubble Tea v2: https://byteiota.com/bubble-tea-v2-10x-faster-terminal-uis-for-go-developers/
- go-github: https://github.com/google/go-github
- Gitea Go SDK: https://gitea.com/gitea/go-sdk
- Binary size comparison (Deno→Bun): https://zenn.dev/dyoshikawa/articles/deno-to-bun-single-binary?locale=en
- Vim keybinding blog: https://sngeth.com/opencode/vim/typescript/solidjs/tui/2026/03/11/adding-vim-keybindings-to-opencode/
