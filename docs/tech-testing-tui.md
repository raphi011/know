# Testing the Bubbletea v2 TUI

## Why not teatest?

Charmbracelet's `teatest` package (`github.com/charmbracelet/x/exp/teatest`) wraps a `tea.Program` with fake stdin/stdout for integration-style testing. However, it depends on bubbletea v1 and is not compatible with v2. We follow the approach used by Charmbracelet's own projects (e.g. Crush): test components directly without terminal emulation.

## Approach

### 1. Test pure rendering functions directly

Most rendering logic lives in package-level functions that take explicit args and return strings. Test these with `strings.Contains` assertions on the output — not golden files, since ANSI escape codes from lipgloss change across versions.

```go
func TestRenderToolStatus(t *testing.T) {
    p := ContentPart{
        Type:     PartToolCall,
        ToolName: "search_documents",
        Input:    map[string]any{"query": "test"},
        Status:   ToolRunning,
    }
    got := renderToolStatus(p)
    if !strings.Contains(got, "search_documents") {
        t.Errorf("expected tool name, got %q", got)
    }
}
```

### 2. Test state transitions via a minimal Model

Construct a `Model` with only the fields needed for the test — no real Client, no glamour renderer. Call methods directly (`appendText`, `handleStreamEvent`, `finalizeStream`) and inspect the resulting fields.

```go
func testModel() Model {
    return Model{
        ready: true, termReady: true,
        width: 80, height: 24,
        input: textinput.New(),
        ctx:   context.Background(), cancel: func() {},
    }
}

func TestAppendText(t *testing.T) {
    m := testModel()
    m.appendText("hello ")
    m.appendText("world")
    // Text coalesces into one PartText
    if m.streamParts[0].Content != "hello world" {
        t.Errorf("got %q", m.streamParts[0].Content)
    }
}
```

### 3. Test Update() by calling handlers with synthetic messages

`handleStreamEvent` and `handleKey` return `(tea.Model, tea.Cmd)`. Type-assert the result to `Model` and check state. Commands are checked for nil vs non-nil — we can't execute them without a running program, but nil-ness tells us whether side effects were triggered (e.g. `finalizeStream` returns `tea.Println`, so `msg_end` should yield a non-nil cmd).

```go
func TestHandleStreamEvent_MsgEnd(t *testing.T) {
    m := testModel()
    m.streaming = true
    m.streamParts = []ContentPart{{Type: PartText, Content: "response"}}
    msg := streamEventMsg{
        event: StreamEvent{Type: "msg_end", InputTokens: 1500},
    }
    result, cmd := m.handleStreamEvent(msg)
    rm := result.(Model)
    if rm.streaming {
        t.Error("expected streaming=false")
    }
    if cmd == nil {
        t.Error("expected non-nil cmd (tea.Batch with finalize)")
    }
}
```

## What NOT to test

- **Async commands** (`createConversation`, `listenForEvents`, and the inner streaming closures of `sendMessage`): thin wrappers around the REST client. Testing them requires an HTTP mock server for little value.
- **`View()` composition**: it composes sub-renders with conditional visibility logic (ready/streaming/dialog/error checks); the conditions are implicitly covered by state machine tests. Full View output is fragile without adding value.
- **Framework behavior**: spinner ticks, textinput cursor blink, viewport scrolling — trust bubbletea.
- **`NewModel` constructor**: wires up dependencies, not worth testing.

## Why not golden files?

Golden files (`testdata/*.golden`) store expected ANSI output and compare against it. They're useful for visual regression testing but fragile in our case:

- Lipgloss and glamour embed ANSI escape codes that change across versions
- Style changes (colors, padding) cause golden file churn unrelated to logic bugs
- `strings.Contains` on key fragments (tool names, role labels, diff markers) catches real regressions without the maintenance burden

## Test file organization

| File | Tests |
|------|-------|
| `tasks_test.go` | Pure helpers: `shortDocPath`, `isOverdue`, `highlightRunes` |
| `render_test.go` | Rendering: `formatTokens`, `renderStatusBar`, `renderContextBar` |
| `app_test.go` | State machine: `appendText`, `findToolPart`, `updateToolStatus`, `handleStreamEvent` (text, tool lifecycle, tool reuse, error, conv_id, interrupted, msg_end, done), `finalizeStream`, `toolKeyArg`, `toolDetail`, `renderToolStatus` |
| `attachment_test.go` | File classification and resolution |
| `filelist_test.go` | File list CRUD and navigation |
