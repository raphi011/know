# Bubbletea v2 Best Practices & Gotchas

Reference guide for building TUIs with bubbletea v2, bubbles v2, and lipgloss v2.

## Import Paths (v2)

```go
import (
    tea "charm.land/bubbletea/v2"
    "charm.land/bubbles/v2/viewport"
    "charm.land/bubbles/v2/textinput"
    "charm.land/bubbles/v2/list"
    lipgloss "charm.land/lipgloss/v2"
)
```

## v2 Breaking Changes Cheat Sheet

| v1 | v2 |
|----|-----|
| `View() string` | `View() tea.View` via `tea.NewView(s)` |
| `tea.KeyMsg` | `tea.KeyPressMsg` |
| `msg.Type == tea.KeySpace` | `msg.String() == "space"` |
| `msg.Alt` | `msg.Mod.Contains(tea.ModAlt)` |
| `msg.Runes` | `msg.Text` (string) |
| `msg.Type` | `msg.Code` (rune) |
| `tea.KeyCtrlC` | `msg.String() == "ctrl+c"` |
| `tea.WithAltScreen()` | `view.AltScreen = true` |
| `tea.WithMouseCellMotion()` | `view.MouseMode = tea.MouseModeCellMotion` |
| `tea.SetWindowTitle("x")` cmd | `view.WindowTitle = "x"` |
| `tea.MouseMsg` direct fields | `tea.MouseClickMsg` etc., call `.Mouse()` |
| `tea.Sequentially()` | `tea.Sequence()` |
| `tea.WindowSize()` | `tea.RequestWindowSize` |
| `spinner.Tick()` package func | `model.Tick()` method |

### Bubbles v2

| v1 | v2 |
|----|-----|
| `viewport.New(w, h)` | `viewport.New(viewport.WithWidth(80))` |
| `vp.YOffset` field | `vp.SetYOffset()` / `vp.YOffset()` |
| `vp.HighPerformanceRendering` | Removed (Cursed Renderer handles it) |
| `textinput.NewModel()` | `textinput.New()` |
| `ti.PromptStyle` | `ti.Styles.Focused.Prompt` |
| `ti.Cursor` field | `ti.Cursor()` method → `*tea.Cursor` |
| `help.DefaultKeyMap` var | `help.DefaultKeyMap()` func |
| `DefaultStyles()` | `DefaultStyles(isDark bool)` |

### Light/Dark Detection

```go
// In Init() — non-blocking, works over SSH
func (m Model) Init() tea.Cmd {
    return tea.RequestBackgroundColor
}

// In Update()
case tea.BackgroundColorMsg:
    m.isDark = msg.IsDark()
    m.styles = newStyles(m.isDark)

// Quick alternative (blocking, no SSH support)
isDark := compat.HasDarkBackground()
```

## Architecture Patterns

### Keep the Event Loop Fast

```go
// GOOD — offload work to Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    case submitMsg:
        return m, m.doExpensiveWork // runs in goroutine
}

// BAD — blocks the event loop
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    case submitMsg:
        result := doExpensiveWork() // blocks rendering
        m.result = result
        return m, nil
}
```

`View()` must be a **pure render function** — no side effects, no I/O.

### Model Composition (Parent → Children)

```go
type App struct {
    sidebar  SidebarModel
    content  ContentModel
    active   pane
    width, height int
}

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        // Broadcast to ALL children
        m.sidebar.SetSize(leftW, msg.Height)
        m.content.SetSize(rightW, msg.Height)

    case tea.KeyPressMsg:
        // Global keys first
        if key.Matches(msg, m.keys.Quit) {
            return m, tea.Quit
        }
        // Route to active child
        switch m.active {
        case paneSidebar:
            return m, m.sidebar.handleKey(msg)
        case paneContent:
            return m, m.content.handleKey(msg)
        }
    }
}
```

**Key rules:**
- Root handles global keys, delegates domain keys to active child
- Broadcast `WindowSizeMsg` to all children (not just active)
- Children communicate via custom messages, not direct references

### SSE / Channel Streaming

Pattern for integrating server-sent events with bubbletea:

```go
// 1. Start stream — returns channel
func startStream() tea.Cmd {
    return func() tea.Msg {
        ch, err := client.Stream(ctx)
        if err != nil {
            return streamErrMsg{err}
        }
        return streamStartMsg{ch: ch}
    }
}

// 2. Listen for next event — chain Cmds
func listenStream(ch <-chan Event) tea.Cmd {
    return func() tea.Msg {
        event, ok := <-ch
        if !ok {
            return streamDoneMsg{}
        }
        return streamEventMsg{event: event, ch: ch}
    }
}

// 3. In Update — process + chain
case streamEventMsg:
    m.handleEvent(msg.event)
    return m, listenStream(msg.ch) // chain next read

case streamDoneMsg:
    m.streaming = false
    return m, m.loadFinalState() // reload after stream ends
```

**Important:** pass the channel through each message so the next Cmd can read from it.

### Batch vs Sequence

```go
// Concurrent — independent operations
cmd := tea.Batch(fetchUsers, fetchSettings, startTimer)

// Serial — order matters or results depend on each other
cmd := tea.Sequence(saveFile, reloadView)
```

### Layout Arithmetic

```go
// GOOD — measure rendered content dynamically
header := m.renderHeader()
footer := m.renderFooter()
contentH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)

// BAD — hardcoded magic numbers
contentH := m.height - 3
```

Use `lipgloss.Height()` and `lipgloss.Width()` to measure rendered strings.

## Styling Best Practices

### Style Functions Over Variables

```go
// GOOD — supports dynamic themes
func ActiveStyle() lipgloss.Style {
    return lipgloss.NewStyle().Foreground(theme.Primary)
}

// OK for static styles
var borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
```

### Theme Detection & Initialization

```go
// Initialize theme before starting program
profile := colorprofile.Detect(os.Stderr, os.Environ())
p := tea.NewProgram(model,
    tea.WithOutput(os.Stderr),
    tea.WithColorProfile(profile),
)
```

### Stderr for TUI Output

```go
// Enables: cd $(mytool select-dir)
// TUI renders to stderr, result prints to stdout
p := tea.NewProgram(model, tea.WithOutput(os.Stderr))
```

## Common Gotchas

### 1. Message Ordering is NOT Guaranteed

Commands run in goroutines — completion order is unpredictable. Only user input maintains order.

**Fix:** Use `tea.Sequence()` when order matters, or design handlers to be order-independent.

### 2. Never Mutate Model from Goroutines

```go
// BAD — race condition with View()
go func() {
    m.data = fetchData()
}()

// GOOD — send message back through event loop
func fetchCmd() tea.Msg {
    data := fetchData()
    return dataMsg{data}
}
```

### 3. Panics in Commands Don't Recover Terminal

Only event-loop panics trigger terminal recovery. A panic inside a `tea.Cmd` goroutine leaves the terminal in raw mode.

**Fix:** Run `reset` in terminal. In production, add panic recovery in Cmds.

### 4. SIGINT/SIGQUIT Must Be Handled Manually

v2 doesn't auto-handle signals. Add explicit handling in `Update()`:

```go
case tea.KeyPressMsg:
    if msg.String() == "ctrl+c" {
        return m, tea.Quit
    }
```

### 5. Hot Reload Tools Don't Support TTY

Live-reload tools like `air` don't support TTY programs. Use `watchexec` with separate build/run scripts instead, or just rebuild manually with `just dev`.

### 6. Focus Must Be Deferred Until Terminal Setup Completes

`textinput.Focus()` is a **pointer receiver** that sets `m.focus = true` and returns a cursor blink `tea.Cmd`. When not focused, `textinput.Update()` silently drops all messages — the input appears frozen.

Two issues with early focus:

**Problem 1 — `Init()` value copy**: `Init()` operates on a copy of the model, so `Focus()` mutations are lost.

**Problem 2 — Terminal query responses**: Bubbletea v2 sends DECRQM and OSC queries on startup. If the textinput is focused before these responses are consumed, the escape sequences can appear as typed garbage (e.g. `]11;rgb:3030/3434/4646[?2026;2$y`).

**Fix:** Defer focus until `WindowSizeMsg` arrives (indicates terminal setup is done) AND any other readiness conditions are met:

```go
func NewModel() Model {
    ti := textinput.New()
    // Do NOT focus here — terminal queries haven't been sent yet
    return Model{input: ti}
}

func (m Model) Init() tea.Cmd {
    return m.loadData() // no focus cmd
}

case tea.WindowSizeMsg:
    if !m.termReady {
        m.termReady = true
        return m, m.tryFocus()
    }

func (m *Model) tryFocus() tea.Cmd {
    if m.termReady && m.ready {
        return m.input.Focus() // safe — terminal queries consumed
    }
    return nil
}
```

Also remember: `Blur()` does NOT return a Cmd — just call it directly.

### 7. Never Use `glamour.WithAutoStyle()` or `termenv.HasDarkBackground()` Inside Update()

`glamour.WithAutoStyle()` calls `termenv.HasDarkBackground()` which reads **directly from `/dev/tty`** — the same fd bubbletea's `TerminalReader` is reading. This causes a data race that can split escape sequences, producing garbage in the textinput.

The race: termenv steals bytes meant for `TerminalReader`, causing it to timeout mid-sequence and emit partial escape codes as individual `KeyPressMsg` characters.

**Fix:** Detect dark/light background **before** `p.Run()` starts, then use `glamour.WithStandardStyle()`:

```go
// cmd_ui.go — BEFORE bubbletea starts
isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
model := NewModel(client, vaultID, isDark)

// model constructor
glamourStyle := "light"
if isDark {
    glamourStyle = "dark"
}
r, _ := glamour.NewTermRenderer(
    glamour.WithStandardStyle(glamourStyle), // NOT WithAutoStyle()
    glamour.WithWordWrap(width),
)

// updateRenderer() — reuses pre-detected style, no TTY read
func (m *Model) updateRenderer() {
    r, err := glamour.NewTermRenderer(
        glamour.WithStandardStyle(m.glamourStyle), // safe
        glamour.WithWordWrap(m.width - 4),
    )
}
```

Ref: [bubbletea#1590](https://github.com/charmbracelet/bubbletea/issues/1590)

### 8. Viewport Content Accumulation

Viewports store all content in memory. For long-running sessions (chat apps), content can grow unbounded.

**Fix:** Implement pagination or a sliding window for message history. Or use inline mode with `tea.Println` scrollback instead of a viewport.

### 9. Glamour Renderer and Window Resize

`glamour.NewTermRenderer()` is moderately expensive. Don't recreate on every `WindowSizeMsg`.

**Fix:** Cache the renderer and only recreate when width actually changes.

### 10. Two-Phase ESC Pattern

Better UX: first ESC clears input, second ESC cancels/quits.

```go
case tea.KeyPressMsg:
    if msg.String() == "esc" {
        if m.input.Value() != "" {
            m.input.SetValue("")
            return m, nil
        }
        return m, tea.Quit // or navigate back
    }
```

### 11. AdaptiveColor Removed in v2

`lipgloss.AdaptiveColor` is gone. Use `tea.BackgroundColorMsg` to detect dark/light and select styles accordingly.

## Testing

### teatest — Integration Testing

```go
func TestModel(t *testing.T) {
    m := NewModel()
    tm := teatest.NewTestModel(t, m,
        teatest.WithInitialTermSize(80, 24),
    )

    // Send input
    tm.Send(tea.KeyPressMsg{Code: 'q'})

    // Wait for condition
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("expected"))
    })

    // Assert final state
    fm := tm.FinalModel(t).(Model)
    assert.Equal(t, expected, fm.someField)
}
```

### Golden File Testing

```go
out, _ := io.ReadAll(tm.FinalOutput(t))
teatest.RequireEqualOutput(t, out)
// Update with: go test -v ./... -update
```

**CI tip:** Force ASCII color profile for consistent golden files:
```go
func init() {
    lipgloss.SetColorProfile(termenv.Ascii)
}
```

Add to `.gitattributes`: `*.golden -text`

### Pure Model Testing (No teatest)

Drive `Update()` directly with messages and assert state:

```go
func TestUpdate(t *testing.T) {
    m := NewModel()
    m, cmd := m.Update(someMsg{data: "x"})
    assert.Equal(t, "x", m.(Model).data)
    assert.Nil(t, cmd)
}
```

### Debugging: Message Dump

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if m.debugFile != nil {
        spew.Fdump(m.debugFile, msg)
    }
    // ...
}
```

Tail the debug file during development to see message types, ordering, and timing.

## Patterns from wt Project

The `wt` project implements a **wizard framework** worth studying:

### Step Interface

Each wizard step implements a uniform interface:
- `Init() tea.Cmd` — setup (focus text input, etc.)
- `Update(tea.KeyPressMsg) (Step, tea.Cmd, StepResult)` — only handles key events
- `View() string` — render
- `Value() StepValue` — extract result
- `HasClearableInput() / ClearInput()` — two-phase ESC support

The wizard orchestrator handles navigation (advance/back/skip) and summary display.

### Disabled Options with Auto-Skip

```go
// Cursor automatically skips disabled items
func findNextEnabled(options []Option, from int) int {
    for i := from + 1; i < len(options); i++ {
        if !options[i].Disabled {
            return i
        }
    }
    return from
}
```

### Scroll Indicators for Bounded Lists

```go
if start > 0 {
    sb.WriteString("  ↑ more above\n")
}
// render visible items
if end < len(options) {
    sb.WriteString("  ↓ more below\n")
}
```

## Inline Mode (Non-Alt-Screen)

When `View.AltScreen` is `false` (the default), bubbletea uses **inline mode** with a dynamically-sized managed region.

### How It Works

- `View()` content height **defines** the managed region size — it resizes every frame
- The managed region repaints in-place at the bottom of the terminal
- `tea.Println(...)` returns a `tea.Cmd` that prints text **above** the managed region into permanent terminal scrollback
- On quit, scrollback is preserved — user can scroll up to see everything

### `tea.Println` / `tea.Printf`

```go
// Both return tea.Cmd — usable with tea.Sequence, tea.Batch
tea.Println("message")
tea.Printf("hello %s", name)

// Silent in alt-screen mode — no output produced
// Always prints on its own line with trailing \r\n
```

There are also program-level methods for use from outside the event loop:
```go
p.Println(...)  // blocks until message accepted
p.Printf(...)
```

### Scrollback Chat Pattern

Used for Claude-Code-style inline chat UIs:

```go
// 1. Completed messages → scrollback (permanent)
func (m *Model) sendMessage() tea.Cmd {
    return tea.Sequence(
        tea.Println(renderUserMessage(content)),  // user msg to scrollback
        startStreamCmd,                            // begin SSE stream
    )
}

// 2. Active streaming → View() managed region (repaints in-place)
func (m Model) View() tea.View {
    var content strings.Builder
    if m.streaming {
        content.WriteString(renderStreamParts(m.renderer, m.streamParts, m.pendingApproval, m.width))
    }
    content.WriteString(m.input.View())
    return tea.NewView(content.String())
}

// 3. When stream completes → commit to scrollback
func (m *Model) finalizeStream() tea.Cmd {
    rendered := renderAssistantMessage(m.renderer, m.streamParts)
    m.streamParts = nil
    return tea.Println(rendered)  // View() shrinks back to just input
}
```

No viewport component needed — terminal scrollback handles history.

### Key Differences from Alt-Screen

| Behavior | Inline | Alt-Screen |
|---|---|---|
| Terminal scrollback | Preserved | Replaced (separate buffer) |
| `tea.Println` | Works | No-op |
| Region height | Dynamic (= content height) | Fixed (= terminal height) |
| On quit | Cursor moves to bottom, scrollback preserved | Exits alt buffer, restores main screen |
| Frame > terminal height | Top lines dropped | Should not happen |

### Gotchas

- `WindowSizeMsg` arrives automatically at startup and on SIGWINCH — no need to request it
- Very long streaming responses grow the managed region toward terminal height; top lines are dropped if it exceeds
- Inline mode cursor sits at end of managed region by default; set `View.Cursor` explicitly for text input positioning

### `tea.Println` Clears Previous Terminal Output

**Critical**: `tea.Println` wipes all terminal content above the managed region on its first call. This means any startup banner or welcome message printed via `tea.Println` will destroy the user's previous terminal scrollback.

**Rule**: Never use `tea.Println` during initialization (Init or early Cmd results). Print any startup content with `fmt.Println` **before** calling `p.Run()`.

```go
// BAD — tea.Println in Init or early handler eats all previous output
func (m model) Init() tea.Cmd {
    return tea.Println("Welcome!")  // wipes terminal
}

// BAD — tea.Println in async init result also eats everything
case initDoneMsg:
    return m, tea.Println("Ready!")  // wipes terminal

// GOOD — print before bubbletea starts
func main() {
    fmt.Println(banner())           // safe: before p.Run()
    p := tea.NewProgram(model{})
    p.Run()
}
```

`tea.Println` is safe to use **during interaction** (e.g. committing chat messages to scrollback) — at that point the scrollback is TUI content, not previous terminal output.

### Multi-Line Views Eat Terminal Lines

Each line in `View()` output costs one line of previous terminal scrollback on startup — bubbletea scrolls the terminal to make room. A 3-line view eats 2 lines. Keep the base managed region compact: avoid decorative blank lines, use `\n` not `\n\n`.

All `View()` code paths that transition between each other (e.g. loading → ready) should produce the **same height** to avoid additional eating during state changes.

## Performance Notes

- **Cursed Renderer (v2):** Based on ncurses algorithms, much faster than v1. Handles synchronized output automatically.
- **Auto color downsampling:** v2 adjusts colors to terminal capabilities automatically.
- **Declarative View fields:** Eliminates race conditions from v1's imperative command approach.

## Sources

- [Bubbletea v2 Upgrade Guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)
- [Bubbles v2 Upgrade Guide](https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md)
- [Tips for Building Bubble Tea Programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [Writing Bubble Tea Tests](https://carlosbecker.com/posts/teatest/)
- [The Bubbletea State Machine Pattern](https://zackproser.com/blog/bubbletea-state-machine)
