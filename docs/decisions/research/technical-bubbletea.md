# Bubble Tea v2 — Technical Strengths Research

**Research date:** 2026-06-13
**Framework version:** v2.0.x (`charm.land/bubbletea/v2`)
**Primary sources:** Official release notes, pkg.go.dev, DeepWiki, HN discussions, blog posts

---

## Architecture & Rendering Model

### The Elm Architecture (Model-View-Update)

Bubble Tea implements the Elm Architecture, known as Model-View-Update (MVU). Every program satisfies three methods on a `Model` interface:

```go
type Model interface {
    Init() tea.Cmd                        // startup command, or nil
    Update(tea.Msg) (tea.Model, tea.Cmd)  // pure state transition
    View() tea.View                       // pure render
}
```

The `Program` struct runs a **serial event loop**: it reads one message at a time from an unbuffered channel, calls `Update()`, queues the returned command, and calls the renderer. Because the loop is single-threaded, `Update()` is **never called concurrently** — no mutexes or locks are needed in application code.

### What Changed in v2 vs v1

**Import path:** `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2`

**View return type (major API change):** `View()` now returns a `tea.View` struct rather than a `string`. This enables declarative configuration of terminal features directly from the view:

```go
func (m model) View() tea.View {
    v := tea.NewView("your content here")
    // Alt-screen, mouse mode, cursor, window title — all set on the View struct
    return v
}
```

Imperative commands like `EnterAltScreen` and `EnableMouseCellMotion` are gone; their equivalents are now fields on the `View` struct.

**Key message restructuring:** `tea.KeyMsg` is replaced by `tea.KeyPressMsg` and `tea.KeyReleaseMsg`. The `key.Type`/`key.Runes` fields became `key.Code`/`key.Text`. Space bar now returns `"space"` rather than `" "`. The spacebar distinction matters for keybinding matching.

**Mouse messages:** Unified `MouseMsg` split into `MouseClickMsg`, `MouseReleaseMsg`, `MouseWheelMsg`, `MouseMotionMsg`.

**Lip Gloss unification:** Previously Bubble Tea and Lip Gloss fought over I/O (keyboard vs. color queries). In v2, Bubble Tea owns all I/O and Lip Gloss is a pure styling library.

### The Cursed Renderer

v2 ships a completely new renderer built from scratch, based on the ncurses cell-diffing algorithm. Key properties:

- **Cell-based diffing:** Only changed terminal cells are re-emitted, reducing both bandwidth and flicker.
- **Ultraviolet library:** The underlying screen-buffer and diff engine is extracted into a separate `ultraviolet` package for testability and reuse.
- **Significant SSH/Wish bandwidth reduction:** Orders of magnitude fewer bytes sent over SSH sessions (the ncurses approach is well-suited to high-latency links).

No public benchmark numbers (CPU/memory) have been published by Charm. Independent testing comparing Bubble Tea (Go) vs. Ratatui (Rust) found Ratatui used 30-40% less memory and 15% lower CPU on a 1,000 data-points/second dashboard test, attributable primarily to Rust's lack of a garbage collector. For a PR inbox tool (infrequent updates, moderate item counts), Go's GC is not a practical problem.

### Mode 2026 — Synchronized Updates (Anti-Tearing)

Mode 2026 ("Synchronized Output" / "Batched Rendering") is **enabled by default** in v2. When active, the terminal buffers all escape sequences from a single frame and applies them atomically, eliminating screen tearing that occurs when the terminal renders mid-update.

Supported terminals: Ghostty, Kitty, Alacritty, iTerm2, Foot, WezTerm, Rio, Contour. Falls back gracefully on unsupported terminals.

### Mode 2027 — Wide Unicode

Also auto-enabled where supported. Allows correct layout of wide Unicode characters and emoji without developer intervention.

### Progressive Keyboard Enhancements

On modern terminals (Ghostty, Kitty, WezTerm, etc.), v2 can detect:
- Previously unmappable combinations: `shift+enter`, `ctrl+m` vs. backspace disambiguation, `super+space`
- Key release events (useful for games, not for a PR tool)
- Repeated key state (`key.IsRepeat`)

### Frame Rate

Default: **60 FPS**. Configurable via `tea.WithFPS(n)`, capped at 120 FPS:

```go
p := tea.NewProgram(model, tea.WithFPS(60))
```

The renderer only redraws when there is actual model state change; frames without changes are skipped.

---

## Multi-Panel Layout Ergonomics

### Nested Submodel Pattern

The standard pattern is embedding child models as fields in the parent struct:

```go
type appModel struct {
    activePane  int              // 0 = list, 1 = detail
    listPane    list.Model       // from charmbracelet/bubbles
    detailPane  viewport.Model   // from charmbracelet/bubbles
    width, height int
}
```

The parent's `Update()` inspects application state and delegates messages to the active child:

```go
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch msg.String() {
        case "tab":
            m.activePane = (m.activePane + 1) % 2
            return m, nil
        case "j", "down":
            if m.activePane == 0 {
                // route to list
            }
        }
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        // resize both panes
    }

    var cmd tea.Cmd
    switch m.activePane {
    case 0:
        m.listPane, cmd = m.listPane.Update(msg)
        // sync detail pane when list selection changes
        m.detailPane.SetContent(m.listPane.SelectedItem().(prItem).body)
    case 1:
        m.detailPane, cmd = m.detailPane.Update(msg)
    }
    return m, cmd
}
```

**Layout with Lip Gloss:** The standard approach uses `lipgloss.JoinHorizontal` or vertical join to compose pane strings side by side. The parent `View()` calls each child's `.View()` and joins them:

```go
func (m appModel) View() tea.View {
    left  := lipgloss.NewStyle().Width(m.width/2).Render(m.listPane.View())
    right := lipgloss.NewStyle().Width(m.width - m.width/2).Render(m.detailPane.View())
    return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
}
```

### Focus/Blur Mechanics

Bubbles components (`list.Model`, `textinput.Model`, `textarea.Model`) expose `.Focus()` and `.Blur()` methods. Only the focused component consumes character input. The routing convention:

1. Track active pane in model state (`m.activePane int` or an enum).
2. On tab/keybinding: call `.Blur()` on current, `.Focus()` on next, update state.
3. In `Update()`, delegate key messages only to the active pane.

`tea.FocusMsg` / `tea.BlurMsg` are system messages (terminal window focus, not app-internal pane focus) — do not confuse them with the `.Focus()` / `.Blur()` methods on Bubbles components.

### pug (Terraform TUI) — Real Multi-Panel Example

PUG implements a **three-pane layout** (explorer left, content top-right, tasks bottom-right) with:
- Numeric hotkeys (`0`, `1`, `2`) and `tab` for pane cycling
- `+`/`-`/`<`/`>` for dynamic pane resizing
- Separate history stack for the top-right pane
- Global keybindings (`?`, `ctrl+c`, `esc`) handled at root before delegation

This is the most architecturally complete public Bubble Tea example studied. Source: [github.com/leg100/pug](https://github.com/leg100/pug)

### Known Layout Pain Points

- **Height arithmetic:** Borders consume 2 rows; forgetting to subtract causes content overflow. Must manually account for status bars, help lines, and borders in every height calculation.
- **Window resize:** `tea.WindowSizeMsg` arrives asynchronously after startup; all components must handle it or risk rendering with zero dimensions.
- **Lip Gloss `Height()`/`Width()`:** Use these to measure rendered component size dynamically rather than hardcoding offsets — avoids the arithmetic problem above.
- **No framework layout engine:** Unlike tview's `Flex`/`Grid`, Bubble Tea provides no built-in layout primitives. Layout is manual lipgloss composition. This is flexible but verbose.

---

## Concurrent/Live Update Story

### The Event Loop Guarantee

The serial event loop processes one message at a time. `Update()` is never called concurrently. Model state is only ever modified inside `Update()`. This eliminates the need for mutexes in application code and guarantees no race conditions in model state.

The `msgs` channel is **unbuffered**, providing backpressure: producers block until the event loop receives the message, preventing message loss under burst load.

### Command Execution Model

Commands (`tea.Cmd`) are `func() tea.Msg` values. Each returned command is dispatched into its own goroutine by the framework's command handler:

```
Update() returns Cmd
  → framework spawns goroutine
  → goroutine calls Cmd()
  → result Msg sent to p.msgs channel
  → event loop picks it up next iteration
```

The golden rule: **never mutate model state from inside a Cmd function**. Commands only return messages; all mutations happen in `Update()`.

### tea.Batch — Parallel Commands

```go
return m, tea.Batch(fetchGitHub(), fetchGitLab(), fetchGitea())
```

All three commands execute concurrently in separate goroutines. Results arrive in **non-deterministic order**. This is exactly the right pattern for multi-provider polling in prsm.

### tea.Sequence — Ordered Commands

```go
return m, tea.Sequence(cmd1, cmd2, cmd3)
```

Runs commands one at a time in order, each waiting for the previous to complete. Useful for operations where later steps depend on earlier results.

### tea.Tick / tea.Every — Polling

For polling every N seconds:

```go
type tickMsg time.Time

func tickCmd() tea.Cmd {
    return tea.Tick(60*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case tickMsg:
        return m, tea.Batch(
            fetchAllProviders(),  // kick off concurrent fetches
            tickCmd(),            // schedule next tick
        )
    }
    // ...
}
```

`tea.Tick` fires once; `tea.Every` syncs to the system clock boundary. For a 60-second polling interval, either works. The pattern is to return the next tick command alongside the fetch command so polling is self-sustaining.

### program.Send() — External Goroutine Injection

For background goroutines that need to send messages outside the normal command lifecycle:

```go
p := tea.NewProgram(model)
go func() {
    for update := range providerStream {
        p.Send(providerUpdateMsg{data: update})
    }
}()
p.Run()
```

`p.Send()` is thread-safe. It blocks until the event loop receives the message, and returns immediately if the program context is cancelled.

### The tea.Listen Gap

Currently, `tea.Cmd` terminates after returning one message. For a long-lived goroutine that streams multiple messages (e.g., a WebSocket feed), the standard workaround is the "re-queue yourself" pattern: after each message, return the listening command again.

A `tea.Listen(func(chan<- tea.Msg)) tea.Cmd` API was proposed in [issue #1135](https://github.com/charmbracelet/bubbletea/issues/1135) to address this elegantly. As of the research date, it was not yet in the released API. For prsm's polling use case (60-second intervals), the re-queue tick pattern is sufficient and idiomatic.

### Multi-Provider Race Safety

When three provider goroutines (GitHub, GitLab, Gitea) all return data simultaneously:

1. Each goroutine puts its result message on `p.msgs`.
2. The unbuffered channel serializes them — only one gets picked up at a time.
3. `Update()` handles them sequentially.
4. No race condition in model state is possible.

The arrival order is non-deterministic, but for a PR list display this is fine. Each provider's result can be merged into the model independently.

### Known v2 Race Condition (Internal)

A shutdown-time data race was reported in [issue #1599](https://github.com/charmbracelet/bubbletea/issues/1599): a write in `cancelreader.Close()` racing against a read in `StreamEvents` with no happens-before guarantee. This is an internal framework bug that does not affect application code. It was filed against v2 and is being addressed.

---

## Vim Keybinding Support

### key.Binding / key.Map Pattern

The `charmbracelet/bubbles/key` package provides the canonical keybinding abstraction:

```go
type keyMap struct {
    Up     key.Binding
    Down   key.Binding
    Left   key.Binding
    Right  key.Binding
    Filter key.Binding
    Help   key.Binding
    Quit   key.Binding
    Open   key.Binding
}

var defaultKeys = keyMap{
    Up: key.NewBinding(
        key.WithKeys("k", "up"),
        key.WithHelp("k/↑", "move up"),
    ),
    Down: key.NewBinding(
        key.WithKeys("j", "down"),
        key.WithHelp("j/↓", "move down"),
    ),
    Left: key.NewBinding(
        key.WithKeys("h", "left"),
        key.WithHelp("h/←", "move left"),
    ),
    Right: key.NewBinding(
        key.WithKeys("l", "right"),
        key.WithHelp("l/→", "move right"),
    ),
    Filter: key.NewBinding(
        key.WithKeys("/"),
        key.WithHelp("/", "filter"),
    ),
    Help: key.NewBinding(
        key.WithKeys("?"),
        key.WithHelp("?", "help"),
    ),
    Open: key.NewBinding(
        key.WithKeys("o", "enter"),
        key.WithHelp("o/enter", "open in browser"),
    ),
}
```

`key.Binding` holds both the trigger keys and human-readable help strings. The `bubbles/help` component auto-renders a help bar from any `key.Map` (a struct with `key.Binding` fields), making `?`-to-show-help essentially free.

### Matching in Update()

```go
case tea.KeyPressMsg:
    switch {
    case key.Matches(msg, m.keys.Up):
        // move up
    case key.Matches(msg, m.keys.Down):
        // move down
    case key.Matches(msg, m.keys.Filter):
        m.filtering = true
        // ...
    }
```

### Modal Keybinding (Normal/Insert/Command Mode)

There is **no built-in modal keybinding system** in Bubble Tea or Bubbles. Vim-style modes must be implemented as application state:

```go
type mode int
const (
    modeNormal mode = iota
    modeFilter
    modeHelp
)

type model struct {
    mode mode
    // ...
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch m.mode {
        case modeNormal:
            switch {
            case key.Matches(msg, m.keys.Filter):
                m.mode = modeFilter
                // activate filter input
            case key.Matches(msg, m.keys.Help):
                m.mode = modeHelp
            }
        case modeFilter:
            if msg.String() == "esc" {
                m.mode = modeNormal
                return m, nil
            }
            // delegate to filter textinput
        }
    }
}
```

This is straightforward to implement and is the documented pattern. The `bubbles/list` component itself ships with a built-in filter mode triggered by `/`, which can serve as the basis or be extended. The `DisableQuitKeybindings()` method on `list.Model` is necessary when embedding the list in a larger app to prevent the list from consuming `esc` and `q` as quit.

### Spawning External Processes

`tea.ExecProcess` (v1) / `tea.Exec` (v2) allows pausing the TUI, running an external command (e.g., `open https://...`), and resuming — useful for "open PR in browser" functionality:

```go
return m, tea.Exec(exec.Command("open", pr.URL), func(err error) tea.Msg {
    return browserOpenedMsg{err: err}
})
```

---

## Performance Characteristics

### Frame Rate

- Default: 60 FPS
- Maximum: 120 FPS (capped by `WithFPS`)
- Minimum: configurable down to 1 FPS for battery/bandwidth conservation
- Frames are **skipped** if no model state changed — the renderer does not blindly redraw at the configured FPS

### Memory and CPU

No official Bubble Tea v2 benchmarks were published. From the independent Go vs. Rust comparison:

- Bubbletea uses ~30-40% more memory than Ratatui on a 1,000 data-points/second dashboard, primarily due to Go's GC.
- For prsm's workload (tens to hundreds of PRs, polling every 60 seconds, no video or high-frequency updates), Go's GC overhead is not a practical bottleneck. The working set fits comfortably in memory.
- The dasroot.net article cites "30% faster rendering" and "40% lower interface update latency" for v2 vs. v1, though these numbers lack methodology.

### Rapid State Updates (3 providers simultaneously)

When multiple providers return simultaneously, each message is serialized through the unbuffered `msgs` channel. The event loop processes them one at a time. Three near-simultaneous messages will be handled in three consecutive `Update()` + `View()` cycles. With a 60 FPS cap, this takes at most ~50ms. For a PR inbox, this is imperceptible.

The Cursed Renderer's cell-diffing approach means that if two of the three updates produce the same visual output (e.g., no new PRs from one provider), virtually no bytes are written to the terminal for that frame.

### Known Performance Bottlenecks

- **Layout arithmetic in `View()`:** Calling `lipgloss.Height()` and `lipgloss.Width()` on every frame can be expensive if called on large strings. The advice is to cache rendered subviews when the model hasn't changed. This requires tracking dirty state manually.
- **Large lists:** The `bubbles/list` component paginates, so rendering 1,000+ items is handled via pagination rather than rendering all at once. Performance should be fine for any realistic PR count.
- **GC pauses:** Theoretically possible but not reported as user-visible in any community discussion. Go's GC latency at this scale (a TUI app with small working set) is typically sub-millisecond.

---

## Developer Ergonomics

### Boilerplate for a Basic Two-Panel App

A minimal but functional list+detail pane app requires roughly:

1. **Model struct** with `list.Model`, `viewport.Model`, width/height, activePane int, and PR data slice (~20 lines)
2. **Init()** returning a batch of provider fetch commands + initial tick (~10 lines)
3. **Update()** handling `WindowSizeMsg`, `KeyPressMsg`, tick messages, and provider result messages (~60-80 lines)
4. **View()** composing the two panes with lipgloss (~20 lines)
5. **Key bindings struct** with `key.Binding` fields (~25 lines)
6. **Provider message types** and fetch commands (~30 lines per provider)

Total: approximately 300-400 lines for a two-panel app with one provider and basic keybindings. This grows roughly linearly with providers added via `tea.Batch`.

### Learning Curve

The Elm Architecture is unfamiliar to most Go developers, who are accustomed to imperative mutation. The key mental shift is: **every state change happens through returning a new model from `Update()`**. Developers report:

- The concept clicks after 1-2 small programs.
- Layout arithmetic is the most consistently frustrating part.
- Debugging requires log-to-file (`tea.LogToFile`) because stdout is occupied.
- VS Code debugging requires headless mode to get a TTY.

### Common Pain Points (from HN discussion, GitHub issues, blog posts)

1. **Nested component state sharing is architecturally awkward.** The canonical tension: centralize state at the top level (risks duplication) vs. keep state in child models (requires getter wrappers). Discussion [#707](https://github.com/charmbracelet/bubbletea/discussions/707) documents this with no clean resolution — developers work around it with getters or flat top-level state.

2. **Sibling-to-sibling communication has no built-in solution.** If the list pane selection needs to update the detail pane, the coordination must happen in the parent `Update()`. This is fine architecturally but requires the parent to know the internal structure of both children.

3. **bubbles/list has opinionated defaults** that interfere when embedded in larger apps. `DisableQuitKeybindings()` must be called to prevent the list component from consuming `esc`/`q`. Built-in filter mode may conflict with custom `/` handling.

4. **No framework layout engine.** Every pixel of the layout is manual lipgloss math. Compared to tview's `Flex`/`Grid`, this is significantly more work.

5. **No stdout logging** — `fmt.Println` from anywhere in the program writes to the terminal underneath the TUI. Must use `tea.LogToFile`.

6. **Hot reload / live reload** is not supported for TUI apps. Development iteration requires manual restart.

7. **Panic recovery** in background goroutines was previously incomplete, leaving the terminal in raw mode. v2 adds comprehensive panic recovery.

### Documentation Quality

- **Official docs** (mintlify.wiki/charmbracelet): cover the architecture, key types, and common patterns. Updated for v2.
- **pkg.go.dev (`charm.land/bubbletea/v2`)**: complete API reference with examples for all major types.
- **Upgrade guide**: available from v1 to v2.
- **Examples directory**: 20+ working examples in the repo covering help menus, spinners, lists, viewports, mouse input, and more.
- **Gap:** No official guide specifically for multi-panel/pane-focus patterns. Community blog posts fill this gap.

### Ecosystem Health

- 38,000+ GitHub stars (as of mid-2025)
- 11,682+ Go packages importing bubbletea
- 10,000+ applications built on the framework
- Active maintainers at Charmbracelet with v2 released in 2025
- `charmbracelet/bubbles`: ready-made components including `list`, `viewport`, `textinput`, `textarea`, `spinner`, `progress`, `paginator`, `help`

---

## Complexity Ceiling

### Most Complex Public Bubble Tea App: pug

[github.com/leg100/pug](https://github.com/leg100/pug) is a full-screen Terraform TUI implementing:
- Three-pane layout with dynamic resizing
- Context-specific keybinding sets per pane
- Task queue with capacity management (defaults to 2× CPU cores)
- Exclusive/blocking task categories preventing concurrent writes
- Event-driven state sync (refreshes triggered by task completion)
- Navigator pattern with model caching to prevent recreation on navigation
- Custom table widget with selection, sorting, filtering

The author's blog post documents the architectural challenges that emerged at this complexity level (see leg100.github.io). Key lessons:

- **Model tree beats flat model** for truly complex apps: the root model routes messages; child models handle specific components.
- **Message ordering non-determinism** requires careful reasoning when multiple async commands can affect the same state.
- **Custom widgets** are necessary when Bubbles components have limitations — the author wrote a custom table because the standard one lacked needed features.
- **teatest** provides end-to-end integration testing by sending keypresses and asserting terminal output.

### Does the Elm Architecture Scale?

Based on pug and other complex examples, the pattern holds up well with discipline:

**Scales well when:**
- State is organized hierarchically (root coordinates, children own their slice)
- Message types are strongly typed and well-named
- Parent `Update()` methods handle global concerns first, then delegate

**Gets painful when:**
- Deep nesting creates long message delegation chains
- Siblings need to share data (no pub/sub; must be mediated by parent)
- Components have conflicting keybinding defaults

**Workarounds that help:**
- Keep the model relatively flat at the top level (centralized data, children receive it as parameters)
- Use `tea.Batch` aggressively to avoid sequential command bottlenecks
- Extract reusable sub-models into packages so they can be tested independently

### Apps That Hit Architectural Limits

- lazysql ([github.com/GusJelly/lazysql](https://github.com/GusJelly/lazysql)) is an actively maintained Lazygit-style SQL TUI built on Bubble Tea. It demonstrates that lazygit-style complexity is achievable.
- One HN commenter reported abandoning Bubble Tea for tview after 30 minutes when needs were simple (file picker, dynamic list). Bubble Tea's overhead is real for small utilities.
- The pug author wrote a custom table widget, suggesting the Bubbles component library has gaps at the high end.

---

## Summary Assessment

Bubble Tea v2 is a strong fit for prsm's requirements with two caveats. The serial event loop with goroutine-based commands is a natural match for multi-provider HTTP polling: `tea.Batch(fetchGitHub(), fetchGitLab(), fetchGitea())` dispatches concurrent fetches and the results arrive safely through the event loop with no synchronization code needed. The `tea.Tick`/re-queue pattern handles 60-second polling cleanly. Vim keybindings via `key.Binding`/`key.Map` are first-class, and the `bubbles/list` and `bubbles/viewport` components provide a solid foundation for the list+detail two-pane layout. The v2 Cursed Renderer with Mode 2026 eliminates the tearing issues that plagued v1, and progressive keyboard enhancements on modern terminals allow the full vim keybinding surface prsm needs. The two caveats are: (1) layout arithmetic is fully manual — the developer must carefully manage heights, borders, and resize events with lipgloss, which adds meaningful boilerplate but is not a blocker; and (2) sibling-pane data synchronization (updating the detail pane when list selection changes) must be handled explicitly in the parent model's `Update()`, a pattern that is well-documented in the community but slightly awkward at the framework level. Overall, Bubble Tea v2 is the proven, well-ecosystemed choice for this kind of tool; its 38k-star adoption and the existence of lazysql and pug as close architectural analogues provide strong evidence it handles prsm's complexity tier without hitting the ceiling.

---

## Sources

- [Release v2.0.0 — charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0)
- [Bubble Tea v2: What's New in v2 — Discussion #1374](https://github.com/charmbracelet/bubbletea/discussions/1374)
- [pkg.go.dev — charm.land/bubbletea/v2](https://pkg.go.dev/charm.land/bubbletea/v2)
- [DeepWiki — Concurrency and Goroutines](https://deepwiki.com/charmbracelet/bubbletea/5.1-concurrency-and-goroutines)
- [DeepWiki — charmbracelet/bubbletea overview](https://deepwiki.com/charmbracelet/bubbletea)
- [DeepWiki — charmbracelet/bubbles Display Components](https://deepwiki.com/charmbracelet/bubbles/3-display-components)
- [Tips for building Bubble Tea programs — leg100.github.io](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [Managing nested models with Bubble Tea — donderom.com](https://donderom.com/posts/managing-nested-models-with-bubble-tea/)
- [Build Your Own TUI Apps with Go and BubbleTea — dasroot.net](https://dasroot.net/posts/2026/03/build-tui-apps-go-bubbletea/)
- [Building Bubbletea Programs — Hacker News discussion](https://news.ycombinator.com/item?id=41369065)
- [State duplication and sibling communication — Discussion #707](https://github.com/charmbracelet/bubbletea/discussions/707)
- [Architecture discussion — Discussion #286](https://github.com/charmbracelet/bubbletea/discussions/286)
- [tea.Listen proposal — Issue #1135](https://github.com/charmbracelet/bubbletea/issues/1135)
- [v2 shutdown race condition — Issue #1599](https://github.com/charmbracelet/bubbletea/issues/1599)
- [pug — Terraform TUI built on Bubble Tea](https://github.com/leg100/pug)
- [lazysql — SQL TUI built on Bubble Tea](https://github.com/GusJelly/lazysql)
- [Go vs. Rust for TUI development — DEV Community](https://dev.to/dev-tngsh/go-vs-rust-for-tui-development-a-deep-dive-into-bubbletea-and-ratatui-2b7)
- [key package — charmbracelet/bubbles](https://pkg.go.dev/github.com/charmbracelet/bubbles/key)
- [bubbles/help example — bubbletea repo](https://github.com/charmbracelet/bubbletea/blob/main/examples/help/main.go)
- [Synchronized Output specification](https://github.com/contour-terminal/vt-extensions/blob/master/synchronized-output.md)
- [Bubbles v2 Upgrade Guide](https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md)
