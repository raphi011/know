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

`air` doesn't support TTY programs. Use `watchexec` with separate build/run scripts instead.

### 6. Focus Cmd Must Be Returned

```go
// textinput.Focus() returns a tea.Cmd (for cursor blink)
// You MUST return it or the cursor won't blink
cmd := m.input.Focus()
return m, cmd

// Blur() does NOT return a Cmd
m.input.Blur()
```

### 7. Viewport Content Accumulation

Viewports store all content in memory. For long-running sessions (chat apps), content can grow unbounded.

**Fix:** Implement pagination or a sliding window for message history.

### 8. Glamour Renderer and Window Resize

`glamour.NewTermRenderer()` is expensive. Don't recreate on every `WindowSizeMsg`.

**Fix:** Cache the renderer and only recreate when width actually changes.

### 9. Two-Phase ESC Pattern

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

### 10. AdaptiveColor Removed in v2

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
