# TUI Per-Token Syntax Highlighting (Archived)

This documents the approach used for per-token `@path` syntax highlighting in the bubbletea v2 text input. The feature was implemented and then replaced with a file list UI, but the techniques are reusable for any per-token coloring in bubbletea.

## Problem

Bubbles v2 `textinput.Model` has no API for per-token styling. The component renders its `View()` as a single styled string with cursor management, padding, and placeholder logic baked in. There's no way to inject custom colors for specific character ranges.

See: [charmbracelet/bubbles#633](https://github.com/charmbracelet/bubbles/issues/633)

Existing solutions like [resterm's RuneStyler fork](https://github.com/unkn0wn-root/resterm) reimplement cursor rendering entirely — heavy and fragile.

## Solution: `lipgloss.StyleRanges()` Overlay

`lipgloss.StyleRanges(s string, ranges ...Range)` (lipgloss v2) applies styles to character ranges in a string, respecting existing ANSI escapes. The key insight: positions are in the **ANSI-stripped** string, so you can overlay colors on top of textinput's already-rendered output without interfering with cursor styling, padding, or placeholder logic.

```go
// View() — the core pattern
func (h *HighlightedInput) View() string {
    base := h.inner.View()               // textinput handles everything
    if h.inner.Value() == "" { return base }
    ranges := h.buildStyleRanges(base)    // find tokens, map positions
    return lipgloss.StyleRanges(base, ranges...)  // paint on top
}
```

### Position Mapping

The challenge is converting token positions in the **value string** (full input text) to positions in the **stripped view string** (only the visible window). The formula:

```
viewPos = promptRuneLen + (tokenRunePos - offset)
```

Where:
- `promptRuneLen` = rune length of the prompt in the stripped view (e.g. `"> "` = 2)
- `offset` = first visible rune index (the scroll position)
- Tokens must be clamped to `[offset, offsetRight]` before mapping

### Offset Tracking

textinput's `offset` and `offsetRight` fields are **private**, so we replicate the overflow algorithm (~30 lines) to track the visible window. Called after every `Update()` and `SetValue()`/`SetCursor()`.

The algorithm (from `textinput.handleOverflow()`):
1. If value fits within width → offset=0, offsetRight=len(value)
2. If cursor moved left of offset → shift window left, compute new offsetRight by measuring rune widths forward
3. If cursor moved right of offsetRight → shift window right, compute new offset by measuring rune widths backward

Uses `rw.RuneWidth()` from `github.com/mattn/go-runewidth` for character width measurement (handles CJK, emoji, etc).

### Path Validation Cache

`pathCache map[string]bool` caches `os.Stat` results to avoid stat-ing the same path on every `View()` call (which happens on every render frame during typing).

Cache invalidation: on `SetValue()`, diff current `parseAtRefs()` against cache keys and evict entries for paths no longer present.

## Wrapper Pattern

The `HighlightedInput` struct delegates all editing to `textinput.Model` and only intervenes in `View()`:

```go
type HighlightedInput struct {
    inner       textinput.Model
    offset      int              // mirrors textinput's private field
    offsetRight int
    pathCache   map[string]bool
}

// All editing methods delegate directly
func (h *HighlightedInput) Value() string  { return h.inner.Value() }
func (h *HighlightedInput) Update(msg) Cmd { /* delegate + sync offsets */ }
func (h *HighlightedInput) View() string   { /* delegate + overlay colors */ }
```

This pattern works for any component where you want to add syntax highlighting without forking the upstream component.

## Limitations

- **Prompt length hardcoded**: `promptRuneLen()` returns 2 for `"> "`. Dynamic prompts would need a different approach (e.g. measuring the stripped prefix before the value content).
- **Offset drift risk**: if upstream changes the overflow algorithm, our replica diverges silently. Tests catch this but it's fragile.
- **Single-line only**: the offset tracking assumes a single-line textinput. Multi-line (textarea) would need a completely different approach.
