# Crush TUI Patterns — Charmbracelet in Production

An analysis of [Charmbracelet's Crush](https://github.com/charmbracelet/crush), an AI coding assistant TUI, showcasing how Charmbracelet's own team builds production-grade terminal UIs with their libraries.

**Stack**: Bubbletea v2, Bubbles v2, Lipgloss v2, Glamour v2, Ultraviolet (custom screen buffer)

---

## 1. Custom Lazy-Loaded List

Crush doesn't use `bubbles/list` — they built a custom `list.List` optimized for chat-style rendering with lazy evaluation and render callbacks.

```go
type List struct {
    width, height int
    items         []Item
    gap           int
    reverse       bool          // bottom-to-top for chat
    focused       bool
    selectedIdx   int           // -1 = no selection
    offsetIdx     int           // first visible item index
    offsetLine    int           // lines scrolled within that item
    renderCallbacks []RenderCallback
}

type RenderCallback func(idx, selectedIdx int, item Item) Item
```

**Why it's elegant**: Items are only rendered when visible. The `offsetIdx` + `offsetLine` combo enables sub-item scrolling (scrolling _within_ a long message). Render callbacks let the parent inject focus/selection styling without the list knowing about those concerns.

```go
// Register highlighting without coupling list to focus logic
l.RegisterRenderCallback(func(idx, selectedIdx int, item Item) Item {
    if idx == selectedIdx {
        return item.WithHighlight(true)
    }
    return item
})
```

**Key detail**: `SetReverse(true)` flips scroll direction — natural for chat where newest items are at the bottom.

---

## 2. Dialog Stack (Modal Overlay System)

A minimal but powerful dialog system using a stack and action-return pattern:

```go
type Dialog interface {
    ID() string
    HandleMsg(msg tea.Msg) Action
    Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor
}

type Overlay struct {
    dialogs []Dialog  // stack — last element = frontmost
}
```

**Pattern**: Only the frontmost dialog receives input. Dialogs return `Action` values (not `tea.Cmd`) — the parent model interprets them:

```go
// Dialog returns intent, not behavior
func (d *Quit) HandleMsg(msg tea.Msg) dialog.Action {
    if key.Matches(msg, d.yesKey) {
        return dialog.ActionQuit{}
    }
    return dialog.ActionClose{}
}

// Parent decides what actions mean
case dialog.ActionQuit:
    cmds = append(cmds, tea.Quit)
case dialog.ActionClose:
    m.dialog.CloseFrontDialog()
```

**Why it's elegant**: Dialogs are completely decoupled from application logic. A `QuitDialog` doesn't know how to quit — it just signals intent. The parent orchestrates. This makes dialogs reusable and testable.

---

## 3. Pre-Rendered Animation with Gradient Cycling

The spinner isn't a simple character rotation — it's a gradient-cycling animation with staggered character entrance, all pre-computed for performance:

```go
const (
    fps              = 20              // 50ms per frame
    maxBirthOffset   = time.Second     // stagger character appearance
    prerenderedFrames = 10             // cache this many frames
)

var availableRunes = []rune("0123456789abcdefABCDEF~!@#$£€%^&*()+=_")
```

**Key ideas**:
- Each character has a random `birthOffset` (0–1s) creating a staggered entrance effect
- Gradient colors are pre-computed using HCL color space blending (perceptually uniform)
- Frames are cached globally via `csync.Map` keyed by a settings hash (xxh3)
- Animation IDs prevent frame messages from reaching the wrong spinner instance

**Animation visibility optimization**: When a spinning message scrolls out of the viewport, its animation is paused. No wasted CPU on invisible spinners.

---

## 4. HCL Color Gradients

Text gradients use perceptually uniform HCL color space (not RGB) for smooth transitions:

```go
func ForegroundGrad(t *Styles, input string, bold bool, color1, color2 color.Color) []string {
    clusters := graphemeClusters(input)  // unicode-aware
    ramp := blendColors(len(clusters), color1, color2)
    for i, c := range ramp {
        style := t.Base.Foreground(c)
        clusters[i] = style.Render(clusters[i])
    }
    return clusters
}

func blendColors(size int, stops ...color.Color) []color.Color {
    // Distribute remainder evenly across segments for smooth gradients
    for i := range numSegments {
        c1 := stopsPrime[i]
        c2 := stopsPrime[i+1]
        for j := range segmentSize {
            t := float64(j) / float64(segmentSize-1)
            c := c1.BlendHcl(c2, t)  // HCL, not RGB
            blended = append(blended, c)
        }
    }
    return blended
}
```

**Why HCL**: RGB blending produces muddy intermediate colors. HCL (Hue-Chroma-Luminance) preserves perceptual uniformity — a red→blue gradient passes through purple, not gray.

Used for: dialog titles, spinner animations, logo rendering.

---

## 5. StyleRanges for Selective Styling

Instead of splitting strings and joining styled fragments, Crush uses `lipgloss.StyleRanges()` to apply styles to specific character ranges:

```go
// Underline a single character in a button without splitting the string
func Button(t *styles.Styles, opts ButtonOpts) string {
    text := style.Padding(0, opts.Padding).Render(opts.Text)
    if opts.UnderlineIndex != -1 {
        text = lipgloss.StyleRanges(text,
            lipgloss.NewRange(
                opts.Padding+opts.UnderlineIndex,
                opts.Padding+opts.UnderlineIndex+1,
                style.Underline(true),
            ),
        )
    }
    return text
}
```

Also used for: text selection highlighting in chat, syntax-highlighted line numbers, focus indicators on specific message parts.

**Why it's elegant**: No string splitting/reassembly. Apply cosmetic overlays on top of already-rendered content.

---

## 6. Centralized Style Struct with Semantic Groups

All styles live in a single `Styles` struct organized by UI domain:

```go
type Styles struct {
    Base      lipgloss.Style
    Muted     lipgloss.Style
    HalfMuted lipgloss.Style
    Subtle    lipgloss.Style

    Header struct {
        Charm, Diagonals, Percentage, Keystroke lipgloss.Style
    }
    Chat struct {
        AssistantFocused, AssistantBlurred lipgloss.Style
        ToolCallFocused, ToolCallBlurred, ToolCallCompact lipgloss.Style
    }
    Dialog struct { /* ... */ }
    Pills  struct { TodoSpinner lipgloss.Style }
}
```

**Consistent icon vocabulary**:
```go
const (
    ToolPending = "●"    ToolSuccess = "✓"    ToolError = "×"
    RadioOn     = "◉"    RadioOff    = "○"
    BorderThin  = "│"    BorderThick = "▌"
    ScrollbarThumb = "┃" ScrollbarTrack = "│"
)
```

**Why it's elegant**: One import, one struct, instant discoverability. New components don't invent their own colors — they pick from existing semantic styles.

---

## 7. Double-Press Cancel with Timer

Escape-to-cancel uses a "press twice to confirm" pattern with a timer:

```go
const cancelTimerDuration = 2 * time.Second

func (m *UI) cancelAgent() tea.Cmd {
    if m.isCanceling {
        // Second press within 2s — actually cancel
        m.isCanceling = false
        m.com.App.AgentCoordinator.Cancel(m.session.ID)
        return nil
    }
    // First press — arm the timer
    m.isCanceling = true
    return tea.Tick(cancelTimerDuration, func(time.Time) tea.Msg {
        return cancelTimerExpiredMsg{}
    })
}

// Timer expired without second press — disarm
case cancelTimerExpiredMsg:
    m.isCanceling = false
```

Simple state machine: `isCanceling` bool + `tea.Tick` timer. No channels, no goroutines.

---

## 8. Pub/Sub Event-Driven UI Updates

Backend services emit typed events. The TUI subscribes and reacts:

```go
case pubsub.Event[message.Message]:
    switch msg.Type {
    case pubsub.CreatedEvent:
        cmds = append(cmds, m.appendSessionMessage(msg.Payload))
    case pubsub.UpdatedEvent:
        cmds = append(cmds, m.updateSessionMessage(msg.Payload))
    case pubsub.DeletedEvent:
        m.chat.RemoveMessage(msg.Payload.ID)
    }
```

**Pattern**: `pubsub.Event[T]` is a generic wrapper carrying `Type` (created/updated/deleted) + `Payload T`. Services publish, TUI subscribes via an event channel that feeds into Bubbletea's message loop.

**Why it's elegant**: The TUI never calls service methods to poll for changes. Events arrive as regular `tea.Msg` values, fitting naturally into the Elm architecture. Backend changes (new message from agent, session renamed) appear instantly.

---

## 9. State Machine with Layout Recalculation

UI state transitions go through a single function that always recalculates layout:

```go
type uiState uint8
const (
    uiOnboarding uiState = iota
    uiInitialize
    uiLanding
    uiChat
)

type uiFocusState uint8
const (
    uiFocusNone uiFocusState = iota
    uiFocusEditor
    uiFocusMain
)

func (m *UI) setState(state uiState, focus uiFocusState) {
    m.state = state
    m.focus = focus
    m.updateLayoutAndSize()  // always recalculate
}
```

**Layout system**: A `uiLayout` struct holds computed rectangles for each region (header, sidebar, chat, editor, status, pills). `generateLayout()` computes them from terminal dimensions, compact mode, and current state.

**Responsive breakpoints**:
```go
compactModeWidthBreakpoint  = 120
compactModeHeightBreakpoint = 30
```

Below these thresholds, the UI switches to compact mode (no sidebar, condensed header).

---

## 10. Ultraviolet Screen Buffer Drawing

Instead of Bubbletea's default string-concatenation `View()`, Crush uses `ultraviolet` for direct screen buffer manipulation:

```go
func (m *UI) View() tea.View {
    v := tea.NewView("")
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    v.WindowTitle = "crush " + home.Short(m.com.Store().WorkingDir())

    scr := uv.NewScreenBuffer(m.width, m.height)
    m.Draw(scr, uv.Rect(0, 0, m.width, m.height))
    v.Body = scr.Render()
    return v
}

// Each component draws to a screen region
func (m *Chat) Draw(scr uv.Screen, area uv.Rectangle)
func (m *Status) Draw(scr uv.Screen, area uv.Rectangle)
func (d *Dialog) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor
```

**Why it's elegant**: Pixel-perfect positioning. No string padding tricks. Components draw into their allocated rectangle without knowing about the overall layout. Overlays (dialogs) can draw on top of existing content.

---

## 11. External Editor Integration

Spawn `$EDITOR` as a subprocess, suspending the TUI:

```go
func (m *UI) openEditor(value string) tea.Cmd {
    tmpfile, _ := os.CreateTemp("", "msg_*.md")
    tmpfile.WriteString(value)

    cmd, _ := editor.Command("crush", tmpfile.Name())
    return tea.ExecProcess(cmd, func(err error) tea.Msg {
        content, _ := os.ReadFile(tmpfile.Name())
        os.Remove(tmpfile.Name())
        return openEditorMsg{Text: string(content)}
    })
}
```

`tea.ExecProcess` handles the full lifecycle: suspend alt screen → run process → restore alt screen → deliver result message.

---

## 12. Cached Message Rendering

Messages cache their rendered output keyed by width to avoid re-rendering on every frame:

```go
type cachedMessageItem struct {
    content string
    width   int
    height  int
}
```

Cache is invalidated when:
- Terminal width changes
- Message content updates (streaming)
- Spinning state toggles (animation start/stop)
- Focus/selection state changes

**Line-by-line styling**: Instead of wrapping entire messages in a lipgloss style (expensive for long content), styles are applied line-by-line:

```go
// Instead of: style.Render(longContent)  // re-parses entire string
// Do:
for _, line := range lines {
    styled = append(styled, style.Render(line))
}
```

---

## 13. Collapsible Content with Click Expansion

Long tool outputs and thinking blocks are truncated with an expand/collapse toggle:

```go
const responseContextHeight = 10  // max visible lines when collapsed

// Truncated content shows hint
if lineCount > responseContextHeight {
    visible := lines[:responseContextHeight]
    hint := fmt.Sprintf("… (%d lines hidden) [click to expand]", lineCount-responseContextHeight)
    return strings.Join(visible, "\n") + "\n" + muted.Render(hint)
}
```

Click handling checks if the click target is within an expandable item and calls `ToggleExpanded()`. The `Animatable` interface lets items that support animation participate in the expand/collapse cycle.

---

## 14. Command Composition Patterns

Crush builds complex async flows by composing `tea.Batch` and `tea.Sequence`:

```go
// Parallel: send message + load history simultaneously
return tea.Batch(
    m.sendMessage(content, attachments...),
    m.loadPromptHistory(),
)

// Sequential: pick file → close dialog → reset cache
return tea.Sequence(
    msg.Cmd(),                                           // read file
    func() tea.Msg { m.dialog.CloseDialog(id); return nil }, // close picker
    func() tea.Msg { fimage.ResetCache(); return nil },      // cleanup
)
```

**Error pattern**: Async commands return `util.ReportError(err)` which displays in the status bar with a TTL. No panics, no retries — the user decides what to do.

---

## Summary of Key Takeaways

| Pattern | Why |
|---|---|
| Custom list over bubbles/list | Lazy rendering, sub-item scrolling, render callbacks |
| Dialog stack with action returns | Decoupled modals, parent orchestrates behavior |
| Pre-rendered animation frames | Performance — no per-frame gradient calculation |
| HCL color blending | Perceptually uniform gradients (no muddy colors) |
| `StyleRanges` for partial styling | No string splitting — overlay styles on rendered text |
| Centralized `Styles` struct | Single source of truth, semantic grouping |
| Double-press cancel | Simple state machine with `tea.Tick` timer |
| Pub/Sub events as `tea.Msg` | Backend changes flow naturally through Elm architecture |
| `setState()` always recalculates layout | No stale layout bugs |
| Ultraviolet screen buffer | Pixel-perfect drawing, proper overlays |
| Cached rendering with invalidation | Avoid re-rendering unchanged content |
| `tea.Batch`/`tea.Sequence` composition | Clean async orchestration without goroutine management |

---

## Improvements for Know's TUI

Concrete patterns to adopt from Crush, ordered by impact.

### High Impact

#### 1. Render Caching

Know re-renders all content every frame — `glamour.Render()` runs on every `View()` call for every text part. With long conversations this becomes expensive.

**Adopt**: Cache rendered output keyed by content hash + width. Invalidate on width change or content update.

```go
type cachedPart struct {
	rendered string
	width    int
	hash     uint64 // xxh3 of raw content
}

func (p *ContentPart) Render(width int) string {
	h := xxh3.HashString(p.Text)
	if p.cache.width == width && p.cache.hash == h {
		return p.cache.rendered
	}
	rendered := glamourRender(p.Text, width)
	p.cache = cachedPart{rendered: rendered, width: width, hash: h}
	return rendered
}
```

This matters most during streaming — each new token triggers `View()`, but only the last part actually changed. Everything above it can be served from cache.

#### 2. Structured Error Messages with TTL

Know uses a bare `errMsg string` that lingers until manually cleared. Errors and warnings look identical.

**Adopt**: Crush's `InfoMsg` pattern with severity and auto-dismiss.

```go
type InfoType uint8
const (
	InfoTypeError InfoType = iota
	InfoTypeWarn
	InfoTypeInfo
	InfoTypeSuccess
)

type InfoMsg struct {
	Type InfoType
	Msg  string
	TTL  time.Duration // 0 = sticky
}
```

Then in `Update()`:
```go
case InfoMsg:
	m.infoMsg = msg
	if msg.TTL > 0 {
		return tea.Tick(msg.TTL, func(time.Time) tea.Msg {
			return clearInfoMsg{}
		})
	}
```

This lets you surface warnings (glamour failures, tool_end mismatches, SSE parse errors) that currently vanish into `slog.Warn` where the user never sees them.

#### 3. Double-Press Cancel

Know has no way to cancel a running agent from the TUI. Crush's double-escape pattern is simple and safe.

```go
func (m *Model) cancelAgent() tea.Cmd {
	if m.isCanceling {
		m.isCanceling = false
		// POST /agent/cancel/{id}
		return m.client.CancelAgent(m.conversationID)
	}
	m.isCanceling = true
	m.infoMsg = InfoMsg{Type: InfoTypeInfo, Msg: "Press ESC again to cancel", TTL: 2 * time.Second}
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return cancelTimerExpiredMsg{}
	})
}
```

#### 4. Collapsible Tool Output

Long tool outputs (e.g., search results, file contents) flood the scrollback. Crush truncates to 10 lines with click-to-expand.

```go
const maxCollapsedLines = 10

func renderToolOutput(output string, expanded bool) string {
	lines := strings.Split(output, "\n")
	if !expanded && len(lines) > maxCollapsedLines {
		visible := strings.Join(lines[:maxCollapsedLines], "\n")
		hint := fmt.Sprintf("… (%d lines hidden)", len(lines)-maxCollapsedLines)
		return visible + "\n" + mutedStyle.Render(hint)
	}
	return output
}
```

### Medium Impact

#### 5. Move File List Keys into keyMap

File list navigation is hardcoded as string comparisons (`"up"`, `"down"`, `"backspace"`). This means they don't appear in help text and can't be remapped.

```go
type keyMap struct {
	// ... existing keys ...
	FileList struct {
		Up     key.Binding
		Down   key.Binding
		Delete key.Binding
		Blur   key.Binding
	}
}
```

#### 6. Organize Styles into Semantic Groups

Know's styles are flat variables. As more features are added (tool details, approval UI, settings), this becomes hard to navigate.

```go
type Styles struct {
	Base    lipgloss.Style
	Muted   lipgloss.Style
	Error   lipgloss.Style

	Chat struct {
		UserRole, AssistantRole, ToolRole lipgloss.Style
		UserMsg, AssistantMsg             lipgloss.Style
	}
	Tool struct {
		Pending, Running, Complete, Error lipgloss.Style
		ApprovalBox                       lipgloss.Style
	}
	Status struct {
		Bar, Detail lipgloss.Style
		ContextLow, ContextMid, ContextHigh lipgloss.Style
	}
	Input struct {
		Prompt lipgloss.Style
	}
}
```

One import, one struct, no guessing which variable styles what.

#### 7. Line-by-Line Styling for Long Content

Know wraps entire messages in `style.Render(content)`. For long assistant responses, lipgloss re-parses the entire ANSI string on every render.

```go
// Instead of:
assistantMsgStyle.Render(longContent)

// Do:
var b strings.Builder
for _, line := range strings.Split(longContent, "\n") {
	b.WriteString(assistantMsgStyle.Render(line))
	b.WriteByte('\n')
}
```

### Lower Priority (Worth Knowing)

#### 8. External Editor for Long Messages

Crush lets users press `ctrl+o` to compose in `$EDITOR`. Useful for multi-paragraph prompts that are awkward in a single-line input.

```go
func (m *Model) openEditor() tea.Cmd {
	f, _ := os.CreateTemp("", "know-msg-*.md")
	f.WriteString(m.input.Value())
	f.Close()
	cmd := exec.Command(os.Getenv("EDITOR"), f.Name())
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		content, _ := os.ReadFile(f.Name())
		os.Remove(f.Name())
		return editorResultMsg{text: string(content)}
	})
}
```

#### 9. Approval Context

Know's approval prompt shows the tool name but not what it's trying to do. Adding a summary of tool arguments helps the user decide.

```go
// Current: "Tool: search_documents — [a]pprove [r]eject"
// Better:  "search_documents(query="kubernetes pods", vault="default") — [a]pprove [r]eject"
```

#### 10. Pub/Sub for Multi-View Sync

If Know ever adds multiple views (conversation list + chat), Crush's `pubsub.Event[T]` pattern would let backend changes propagate to all views without polling. Worth keeping in mind but not needed yet with the current single-pane design.
