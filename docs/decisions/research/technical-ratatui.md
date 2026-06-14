# Ratatui — Technical Strengths Research

**Researched:** 2026-06-13
**Framework version at time of research:** v0.30.1 (latest)
**Source:** https://github.com/ratatui/ratatui (21k stars, 31.3M crates.io downloads, 4,200+ dependent crates)

---

## Architecture & Rendering Model

### Immediate-Mode Rendering

Ratatui uses **immediate-mode rendering**: the entire UI is redrawn from scratch every frame based on the current application state. There are no persistent widget objects, no virtual DOM, and no retained widget tree.

```rust
loop {
    terminal.draw(|f| {
        if state.condition {
            f.render_widget(SomeWidget::new(), layout);
        } else {
            f.render_widget(AnotherWidget::new(), layout);
        }
    })?;
}
```

This is conceptually close to IMGUI (Dear ImGui) but applied to terminal output. Casey Muratori's original IMGUI article is cited as background reading in the official docs.

### How It Differs from Retained-Mode (Bubble Tea)

| | Ratatui (immediate) | Bubble Tea (retained/TEA) |
|---|---|---|
| **Widget state** | Rebuilt each frame from app state | Persistent Model struct, `Update()` returns new Model |
| **Control over loop** | Developer-owned | Framework-owned (`Program.Run()`) |
| **Architecture opinion** | None — toolkit only | Enforced Elm Architecture |
| **State management** | Developer decides | Centralized `Model`, `Update(msg)`, `View()` |
| **Async commands** | Tokio + channels (bring your own) | `tea.Cmd` objects return from Update |

### Double-Buffer Diffing

Despite the "draw everything" mental model, Ratatui does not re-emit every character every frame. It maintains two `Buffer` instances (current and previous), diffs them, and only emits the changed cells to stdout. This is what makes immediate-mode viable at 60 FPS without thrashing the terminal.

> "Only changed characters are sent to the terminal emulator over stdout." — techbytes.app

### Render Loop Structure

The standard render loop is an async Tokio main loop with `tokio::select!` multiplexing three independent streams:

1. **Terminal input events** (via `crossterm::event::EventStream`)
2. **Tick events** — periodic application heartbeat (configurable rate, e.g., 10Hz)
3. **Render events** — frame rate ticker (configurable rate, e.g., 30–60 FPS)

```rust
tokio::select! {
    maybe_event = crossterm_event => {
        // Handle keyboard/mouse input -> send Event::Key(...) to channel
    },
    _ = tick_delay => {
        event_tx.send(Event::Tick).unwrap();
    },
    _ = render_delay => {
        event_tx.send(Event::Render).unwrap();
    },
}
```

The main application loop then receives from the event channel:

```rust
loop {
    let event = tui.next().await?;
    if let Event::Render = event { tui.draw(|f| ui(f, &app))?; }
    update(&mut app, event);
    if app.should_quit { break; }
}
```

Key insight: **render rate is fully decoupled from event rate and from background task update rate**. A 60 FPS render loop will faithfully show the latest state without waiting for any specific event.

### Ratatui vs. Shadow-Buffer (OpenTUI/Zig)

OpenTUI (Zig-based) also uses double-buffered rendering but adds RGBA alpha blending and scissor clipping, enabling effects beyond what terminal cells natively support. Ratatui works within terminal cell constraints — no alpha compositing, no sub-cell rendering. For prsm's use case (a PR list/detail view), this distinction is irrelevant; prsm does not need layered transparency or vector-style graphics.

---

## Multi-Panel Layout Ergonomics

### Layout Primitives

Ratatui's layout system is constraint-based, described as "Flexbox for the terminal." Splits are done with `Layout::horizontal()` / `Layout::vertical()` and `Constraint` variants:

```rust
// Two-panel horizontal split: 30% sidebar + remaining detail pane
let [list_area, detail_area] = Layout::horizontal([
    Constraint::Percentage(30),
    Constraint::Fill(1),
])
.areas(frame.area());

// Render PR list to left panel
frame.render_stateful_widget(pr_list, list_area, &mut list_state);

// Render PR detail to right panel
frame.render_widget(pr_detail, detail_area);
```

`Layout::areas()` (introduced recently) destructures directly into named variables, which keeps split code concise.

### Nested Layouts

Layouts compose naturally. A header bar, body split, and status bar is:

```rust
let [header, body, footer] = Layout::vertical([
    Constraint::Length(1),
    Constraint::Fill(1),
    Constraint::Length(1),
]).areas(frame.area());

let [list_pane, detail_pane] = Layout::horizontal([
    Constraint::Percentage(35),
    Constraint::Fill(1),
]).areas(body);
```

### Border Merging (v0.30)

A common visual problem in multi-panel UIs is doubled borders. Ratatui 0.30 introduced `merge_borders` and `Spacing::Overlap` to handle this cleanly:

```rust
let [left, right] = Layout::horizontal([Constraint::Fill(1); 2])
    .spacing(Spacing::Overlap(1))
    .areas(frame.area());

let left_block = Block::bordered()
    .title("PRs")
    .merge_borders(MergeStrategy::Exact);

let right_block = Block::bordered()
    .title("Detail")
    .merge_borders(MergeStrategy::Exact);
```

Before v0.30, this required custom border sets and manual edge-case handling. The new API handles all corner-joining logic automatically.

### Focus Management

Ratatui does **not** have built-in focus management. The developer tracks focused pane state:

```rust
enum FocusedPane { List, Detail }

struct App {
    focus: FocusedPane,
    // ...
}
```

Tab/Shift-Tab and any focus-switching key are handled in the event loop with a simple match:

```rust
KeyCode::Tab => { app.focus = match app.focus {
    FocusedPane::List => FocusedPane::Detail,
    FocusedPane::Detail => FocusedPane::List,
}},
```

For prsm's use case (two panes, keyboard-driven), this is minimal boilerplate — less than 10 lines. Third-party crates exist for more complex scenarios:
- **`ratatui-interact`** — Tab/Shift-Tab `FocusManager<T>`, mouse click regions, interactive widgets
- **`focusable`** — `#[derive(Focus)]` macro for per-widget `focus()` / `blur()` methods
- **`rat-focus`** (now in `rat-salsa`) — focused on Ratatui-specific focus handling

For prsm with only two panes and vim-style navigation, the manual approach is cleaner.

### Observed in Real Apps

**gitui** (50k+ stars) uses multi-panel layouts for staging, diff, commit log, and branch views. It manages complex state across multiple git operation panes simultaneously, all with Ratatui.

**kdash** (Kubernetes dashboard) — real-time multi-panel displays with concurrent async data across nodes, pods, and resource metrics.

**Yazi** (file manager) — async I/O with preview rendering, multi-pane directory traversal.

---

## Concurrent/Live Update Story

### The Standard Pattern: Tokio MPSC Channels

The canonical Ratatui async pattern (documented in the official async template at https://ratatui.github.io/async-template/) uses `tokio::sync::mpsc`:

```rust
pub struct Tui {
    pub terminal: ratatui::Terminal<Backend<std::io::Stderr>>,
    pub task: JoinHandle<()>,
    pub event_rx: UnboundedReceiver<Event>,
    pub event_tx: UnboundedSender<Event>,
    pub frame_rate: f64,
    pub tick_rate: f64,
}
```

Background tasks (e.g., HTTP polling for GitHub PRs, GitLab MRs) hold a clone of `event_tx` and send typed `Event` variants when data arrives:

```rust
// In a background Tokio task polling the GitHub API
tokio::spawn(async move {
    loop {
        let prs = github_client.fetch_open_prs().await?;
        event_tx.send(Event::PrsUpdated { provider: Provider::GitHub, prs })?;
        tokio::time::sleep(Duration::from_secs(60)).await;
    }
});
```

Multiple provider tasks can each hold a sender clone and fire independently. The render loop receives all updates through the single `event_rx`:

```rust
loop {
    let event = tui.next().await?;  // receives from event_rx
    match event {
        Event::PrsUpdated { provider, prs } => app.update_prs(provider, prs),
        Event::Render => tui.draw(|f| render(f, &app))?,
        Event::Key(key) => handle_keypress(&mut app, key),
        // ...
    }
}
```

This pattern cleanly handles prsm's requirement of **concurrent HTTP polling from multiple providers** (GitHub, GitLab, Gitea, Codeberg) all feeding into one render loop.

### Official Event Enum (from async template)

```rust
#[derive(Clone, Debug)]
pub enum Event {
    Init,
    Quit,
    Error,
    Closed,
    Tick,
    Render,
    FocusGained,
    FocusLost,
    Paste(String),
    Key(KeyEvent),
    Mouse(MouseEvent),
    Resize(u16, u16),
}
```

Custom variants for domain events (PR updates, filter changes) are added alongside these.

### App Structure (from async-template)

```
src/
  main.rs       - #[tokio::main], wires Tui + App
  tui.rs        - Terminal wrapper, event streaming, Tui struct
  action.rs     - Action/Message enum (commands emitted by components)
  app.rs        - App state struct + update logic
  components/   - Per-panel components
    home.rs
  config.rs     - Configuration
```

This is the community-endorsed starting structure. The `ratatui/templates` repo provides `cargo-generate` templates for this.

### Shared State: Arc<Mutex<T>> vs. Single-Owner

For most Ratatui apps, the simplest approach is single-owner state: one `App` struct passed mutably through the event loop, with Tokio background tasks communicating only through channels (not sharing the struct directly). This avoids locking entirely.

When state genuinely needs to be shared across threads (e.g., a shared HTTP client, credentials store), `Arc<Mutex<T>>` is standard. The community recommendation is: **use channels first, reach for `Arc<Mutex>` only when channels don't fit**.

The DeepWiki analysis of the Ratatui codebase documents 11 distinct state management patterns, from simple immutable functions to nested `StatefulWidget` hierarchies. For prsm, the `StatefulWidget` pattern is directly applicable to the PR list (scroll position, selected item) and the component-trait pattern fits the multi-pane structure.

---

## Vim Keybinding Support

### Manual hjkl — Minimal Effort

Basic vim navigation for a list requires only a few lines in the event handler:

```rust
KeyCode::Char('j') | KeyCode::Down => app.select_next(),
KeyCode::Char('k') | KeyCode::Up => app.select_prev(),
KeyCode::Char('g') => app.select_first(),
KeyCode::Char('G') => app.select_last(),
KeyCode::Char('/') => app.enter_filter_mode(),
KeyCode::Char('?') => app.toggle_help(),
KeyCode::Enter | KeyCode::Char('l') => app.open_in_browser(),
KeyCode::Char('h') => app.focus_list(),
```

This is about 10–15 lines of match arms. No library needed for prsm's navigation requirements.

### modalkit + modalkit-ratatui

For full modal editing semantics (Normal/Insert/Visual/Command modes with Vim operator-pending state, counts, registers, etc.):

- **`modalkit`** (https://github.com/ulyssa/modalkit) — a framework for building modal applications with drop-in Vim and Emacs keybinding definitions
- **`modalkit-ratatui`** v0.0.24 — Ratatui integration layer; depends on `ratatui ^0.29.0`

```toml
modalkit-ratatui = "0.0.24"
```

Used in production by **iamb**, a full Matrix messaging client with Vim modal interface. iamb demonstrates that modalkit scales to a complex multi-window application with real network async operations.

modalkit provides:
- Full modal state machine (Normal, Insert, Visual, Command modes)
- Default Vim keybindings via `default_vim_keys()`
- `TextBoxState` with buffer management
- Operator-pending state, count prefixes, marks, registers

For prsm v1, this is **overkill**. prsm needs hjkl navigation and `/` filter — not text editing. Manual keybinding handling is the right call. modalkit would be relevant if prsm later adds inline commenting or inline review actions.

### edtui

**`edtui`** (https://github.com/preiter93/edtui) is a Vim-inspired editor widget, not a navigation framework. It provides Normal/Insert/Visual modes for editing text within a text box widget. Not relevant to prsm's navigation needs (no text editing in v1).

### vim-navigator

A lightweight crate providing `j/k`, `g/G`, `:commands`, `/search`, `n/N` for list navigation. More targeted than modalkit for prsm's exact needs. Worth evaluating as a dependency to avoid implementing search-next/prev logic manually.

### Summary for prsm

| Need | Approach | Library |
|---|---|---|
| `j/k` list navigation | 5 lines in event handler | None |
| `h/l` focus switching | 5 lines in event handler | None |
| `/` filter mode | Mode enum + text input widget | `tui-input` or `tui-textarea` |
| `?` help overlay | Conditional render of help widget | None |
| `o` / Enter open browser | `open` crate + `xdg-open` | `open` crate |
| Full modal editing | N/A for v1 | modalkit (future) |

---

## Performance Characteristics

### Benchmark Data (2026)

From a controlled test comparing implementations of a Prometheus monitoring dashboard on AWS t3.medium, monitoring 5,000 time-series data points at 60 FPS:

| Metric | Ratatui (Rust) | Bubble Tea (Go) | Textual (Python) |
|---|---|---|---|
| CPU usage @ 60 FPS | 2% | 6–8% | 22% |
| Memory footprint | ~12MB | ~45MB | ~120MB |
| Input latency (keypress→UI) | 1.2ms | 4.5ms | 18ms |

From a dashboard rendering 1,000 data points per second:
- Ratatui used **30–40% less memory** than Bubble Tea
- **15% lower CPU footprint** (no garbage collector, zero-cost abstractions)
- **4x faster** burst processing of 100,000 telemetry packets (zero-copy deserialization)

### Why Immediate-Mode Is Not a Problem for prsm

A PR list is a mostly-static dataset with infrequent updates (every 60 seconds). The concern about immediate-mode "wasting work" by redrawing every frame is largely irrelevant because:

1. The double-buffer diff means only changed cells are emitted to the terminal — if nothing changed, almost nothing is written to stdout
2. At 30 FPS, prsm is rendering ~33ms frame budgets; a simple two-panel PR list will render in microseconds
3. CPU stays at 2% or lower for static-ish content at 30–60 FPS

The main cost of immediate-mode for prsm is the developer discipline required to reconstruct the full UI each frame — but since prsm's state is simple (a list of PRs + a selected PR detail), this is not burdensome.

### Handling Rapid Concurrent Updates

The mpsc channel pattern absorbs bursty updates naturally. If multiple provider tasks send updates simultaneously, they queue in the channel and the render loop processes them sequentially before the next `Event::Render` fires. With a 30 FPS render rate and 60-second poll interval, there is no realistic scenario where update throughput becomes a bottleneck for prsm.

---

## Developer Ergonomics

### Boilerplate Assessment

**Ratatui v0.30.0+ simplified setup significantly.** The `ratatui::run()` API introduced in 0.30 handles terminal init and restore:

```rust
fn main() -> Result<()> {
    ratatui::run(|frame| render(frame, &state))
}
```

For an async app with Tokio, the skeleton is slightly more involved but still manageable:

```rust
#[tokio::main]
async fn main() -> Result<()> {
    let mut terminal = ratatui::init();
    let mut app = App::default();
    
    // Spawn provider polling tasks
    let (tx, mut rx) = tokio::sync::mpsc::unbounded_channel();
    tokio::spawn(poll_github(tx.clone()));
    tokio::spawn(poll_gitlab(tx.clone()));
    
    // Main event loop
    loop {
        terminal.draw(|f| render(f, &app))?;
        match rx.recv().await? {
            Event::Key(k) => handle_key(&mut app, k),
            Event::PrsUpdated(prs) => app.prs = prs,
        }
    }
}
```

From zero to a working two-panel app with async polling: approximately 200–300 lines of Rust across 3–4 files, using the component template. This is more setup than Bubble Tea's ~100 lines of Go but less than it was pre-0.30.

### Rust Ownership and TUI State

The Rust ownership model creates specific friction points in TUI development:

1. **Shared state between render and event handler**: Both functions need access to `App`. The simplest pattern is a single-threaded loop where `App` is owned by the loop and passed mutably. No locking needed. Background tasks communicate via channels.

2. **Widget lifetime**: Pre-0.26, widgets were consumed (moved) on render, which meant you couldn't store them. Post-0.26, the preferred pattern is `impl Widget for &MyWidget`, allowing widgets to be stored and rendered from references without transfer of ownership.

3. **String rendering**: All text must be renderable as `Text`/`Line`/`Span`, which requires conversion from API response types. This is mechanical but tedious when normalizing across multiple API providers.

4. **Lifetimes in component traits**: If you adopt the component-trait pattern with a `Box<dyn Component>` collection, you may encounter lifetime annotation requirements. Community workaround: use `Arc<dyn Component + Send + Sync>` or avoid trait objects in favor of enums.

### What Developers Report as Pain Points

From GitHub discussions (#552, #220), Reddit r/rust, and comparative blog posts:

1. **No batteries-included architecture** — Ratatui provides no prescribed structure for large apps. Coming from Bubble Tea, where the Elm loop is built-in, developers must choose their own pattern (TEA, MVC, Flux, component trait) and implement it.

2. **StatefulWidget boilerplate** — scroll position, selection state, and other per-widget persistent state requires a companion `State` struct and explicit threading through render calls.

3. **Focus management is DIY** — no built-in Tab navigation; must track focused pane as application state.

4. **Terminal inconsistencies** — different terminal emulators handle ANSI codes, Unicode widths, and true-color differently. Crossterm mitigates much of this but doesn't eliminate it.

5. **Testing is harder** — immediate-mode UIs are difficult to assert on without running the full render pipeline. Community pattern: separate business logic (PR filtering, sorting) from rendering so logic can be unit-tested without a terminal.

6. **Documentation gaps** — the framework-level docs are good; ecosystem-level patterns (how to structure a large multi-component app) are scattered across blog posts and GitHub discussions.

### Positive Ergonomics

- **Rust compiler as a safety net**: ownership errors at compile time prevent entire classes of bugs (use-after-free, data races) that would surface as runtime crashes in Go or Python.
- **Cargo ecosystem**: `crossterm`, `tokio`, `reqwest`, `serde` are all first-class. Adding a GitHub/GitLab API client is straightforward.
- **`cargo-generate` templates**: `ratatui/templates` provides working async starting points.
- **Active community**: Discord, forum at forum.ratatui.rs, 145 open issues, frequent releases (131 releases).

---

## Complexity Ceiling

### Most Complex Open-Source Ratatui Applications

**gitui** (https://github.com/gitui-org/gitui) — ~50k GitHub stars
The go-to reference for complex Ratatui apps. Manages multi-panel staging (line-level hunk staging), commit history, diff viewing, branch management, stash operations, async git operations, and customizable keybindings. Memory usage: 0.17GB vs competitors' 2.6GB. Demonstrates that complex multi-panel TUIs with async backends are fully achievable.

**iamb** (https://github.com/ulyssa/iamb) — Matrix messaging client for Vim addicts
Most architecturally sophisticated Ratatui app found. Features: multi-window management (Vim-style splits), E2EE, image previews (sixels/Kitty), threads, message editing, async Matrix SDK integration, full modal Vim keybindings via `modalkit`. Demonstrates that modalkit + Ratatui scales to a large, complex, production application.

**Yazi** — async file manager with preview rendering, multiple panes, async I/O; described as "blazing fast."

**bottom** — cross-platform system monitor, continuous multi-source async updates (CPU, memory, disk, network), complex charting widgets, multiple display modes.

**kdash** — Kubernetes dashboard with real-time cluster data, multi-panel with concurrent async API polling across multiple resource types.

**slumber** — HTTP/REST client with request/response management, auth workflows, multi-session state.

**managarr** — TUI for multiple Servarr services (Sonarr, Radarr, etc.), coordinating async state from several HTTP APIs simultaneously.

### Does Immediate-Mode Scale?

The evidence from these applications is **yes, with caveats**:

- The applications above are substantially more complex than prsm will be in v1
- The scaling challenge is **architectural discipline**, not Ratatui's rendering model itself
- Developers must choose a state management pattern early and be consistent — the lack of an enforced architecture means large apps can accumulate ad-hoc patterns
- The community-endorsed component architecture (separate `Component` trait with `handle_events`, `update`, `render`) provides the necessary structure for multi-pane apps

### Observed Architectural Patterns in Complex Apps

Complex Ratatui apps tend to converge on a **Flux-like unidirectional data flow**:
1. Input events and async task results enter through one channel
2. A central `update()` function or component tree processes them, mutating `App` state
3. A render pass reads `App` state to produce the frame

This is essentially TEA without the framework enforcement. The discipline required is real but not prohibitive.

---

## Summary Assessment

Ratatui is a strong fit for prsm's specific requirements. Its immediate-mode rendering model is well-suited to a mostly-static PR list that updates every 60 seconds: the double-buffer diff ensures low stdout chatter, and sub-millisecond render times mean a 30 FPS loop consumes negligible CPU. The multi-panel layout system (v0.30's `Spacing::Overlap` + `merge_borders`) makes a two-pane list/detail layout straightforward with clean visual output. The async story is excellent — the Tokio mpsc channel pattern maps directly to prsm's requirement of concurrent HTTP polling from multiple providers (GitHub, GitLab, Gitea, Codeberg), each running as an independent `tokio::spawn` task feeding a shared event channel. Vim keybindings for prsm's v1 scope (hjkl navigation, `/` filter, `?` help, `o` to open) require approximately 15 lines of match arms with no additional library. The primary costs are Rust's learning curve, the absence of a prescribed architecture (requiring an explicit decision between TEA, MVC, or component patterns), and roughly 200–300 lines of bootstrapping for a working async two-panel app. The existence of production applications like iamb (full modal Vim + async Matrix network layer) and gitui (complex multi-panel async git operations) demonstrates that the complexity ceiling is well above what prsm requires in any plausible future scope.

---

## Sources

- https://ratatui.rs — official documentation
- https://github.com/ratatui/ratatui — repository (v0.30.1)
- https://ratatui.rs/concepts/rendering/ — rendering model deep dive
- https://ratatui.rs/tutorials/counter-async-app/full-async-events/ — async event loop pattern
- https://ratatui.github.io/async-template/02-structure.html — recommended async app structure
- https://ratatui.rs/concepts/application-patterns/component-architecture/ — component architecture pattern
- https://ratatui.rs/concepts/application-patterns/the-elm-architecture/ — TEA pattern in Ratatui
- https://ratatui.rs/recipes/layout/collapse-borders/ — border merging (v0.30)
- https://ratatui.rs/highlights/v030/ — v0.30 changelog
- https://ratatui.rs/highlights/v029/ — v0.29 changelog
- https://deepwiki.com/ratatui/ratatui/4.3-state-management-patterns — 11 state management patterns
- https://github.com/ratatui/ratatui/discussions/220 — community best practices discussion
- https://github.com/ratatui/awesome-ratatui — curated app list
- https://github.com/ulyssa/iamb — Matrix client (complex modalkit + Ratatui example)
- https://github.com/gitui-org/gitui — git TUI (complex multi-panel example)
- https://github.com/ulyssa/modalkit — modalkit crate
- https://crates.io/crates/modalkit-ratatui — modalkit-ratatui v0.0.24
- https://github.com/preiter93/edtui — edtui editor widget
- https://blog.tng.sh/2026/03/go-vs-rust-for-tui-development-deep.html — Ratatui vs Bubbletea deep dive
- https://www.glukhov.org/post/2026/02/tui-frameworks-bubbletea-go-vs-ratatui-rust/ — BubbleTea vs Ratatui comparison
- https://techbytes.app/posts/rust-tui-high-performance-cli-monitoring/ — performance benchmarks (CPU/memory/latency)
- https://github.com/d-holguin/async-ratatui — async Ratatui example with multiple concurrent events
- https://news.ycombinator.com/item?id=47184990 — HN discussion: crossterm + Tokio modal editor
- https://news.ycombinator.com/item?id=41654652 — HN discussion: PR TUI in Rust with Ratatui
