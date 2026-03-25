# Universal Filter Bar + Tasks Tab

## Summary

Add a reusable `FilterBar` component for the browse TUI and a new Tasks tab. The filter bar parses inline `key:value` filter tokens from a single text input, with remaining text used as fuzzy search. Each tab declares which filter keys it supports; unsupported keys are treated as search text.

## Filter Bar Component

### Location

`internal/tui/browse/filterbar.go` — new file.

### Interface

```go
// Config declares which filter keys a tab supports.
type FilterBarConfig struct {
	SupportedKeys []string // e.g. ["status", "label", "due", "from"]
	Placeholder   string   // e.g. "Filter tasks... (status:open label:go)"
}

// Result holds the parsed output of the filter input.
type FilterResult struct {
	Query   string              // free-text portion (fuzzy search)
	Filters map[string][]string // recognized key:value pairs (multi-value)
}

// Filter returns the first value for a key, or empty string.
func (r FilterResult) Filter(key string) string
// FilterAll returns all values for a key.
func (r FilterResult) FilterAll(key string) []string

// FilterBar wraps textinput.Model and parses input on every change.
type FilterBar struct {
	input  textinput.Model
	config FilterBarConfig
	result FilterResult
}

// All methods use value receivers and return (FilterBar, tea.Cmd) to match
// the codebase's sub-model convention (see linksModel, searchModel, etc.).
func NewFilterBar(config FilterBarConfig) FilterBar
func (f FilterBar) Update(msg tea.Msg) (FilterBar, tea.Cmd)
func (f FilterBar) View() string
func (f FilterBar) Result() FilterResult
func (f FilterBar) Focus() (FilterBar, tea.Cmd)
func (f FilterBar) Blur() FilterBar
func (f FilterBar) Value() string
```

### Parsing Rules

1. Split input on whitespace.
2. For each token, check if it matches `key:value` where `key` is in `SupportedKeys`.
3. Matching tokens go into `Filters` as `map[string][]string`. Multi-value keys (e.g. `label:go label:rust`) append to the slice. `Filter(key)` returns the first value (convenience for single-value keys like `status`), `FilterAll(key)` returns all values (for multi-value keys like `label`).
4. Non-matching tokens (including unsupported `key:value` pairs) are joined as the `Query` string.
5. Parsing happens on every input change (no debounce needed — it's local string splitting).

### Per-Tab Filter Keys

| Tab | Supported keys | Notes |
|-----|---------------|-------|
| Tasks | `status`, `label`, `due`, `from` | `status:open\|done`, `due:today\|week\|overdue`, `from:/path` |
| All Files | `label`, `type`, `from` | `type:note\|pdf\|audio` |
| Links | `label`, `host`, `status` | `status:inbox\|archived` replaces current `v` toggle |
| Bookmarks | `label` | minimal filtering |
| Search | `label` | free text goes to search API |
| Tags | (none) | all input is fuzzy match on tag names |

### Rollout to Existing Tabs

Each tab replaces its current `textinput.Model` (or `pick.Model` input) with a `FilterBar`. The tab's `Update` method calls `filterBar.Update(msg)`, then reads `filterBar.Result()` to apply filters.

**Links tab**: Replace the `v` key toggle with filter bar. Keep `v` as a shortcut that pre-fills/clears `status:archived` in the filter bar for ergonomic access. Default (no status filter) shows inbox (existing behavior).

**All Files tab**: Currently uses `pick.Model` for fuzzy matching. Replace the input portion with `FilterBar`; keep `pick.Model`'s fuzzy matching logic for the `Query` portion. Add label and type filtering to the API call.

**Bookmarks tab**: Currently has no text input. Add `FilterBar` with `label` support.

**Search tab**: Keep the existing debounced search behavior. Wrap the input with `FilterBar` to extract `label:` filters, pass remaining query to the search API.

**Tags tab**: Use `FilterBar` with no supported keys — all input is fuzzy match on tag names (current behavior preserved).

## Tasks Tab

### Location

`internal/tui/browse/tasks.go` — new file.

### Data Model

```go
type tasksModel struct {
	allTasks []apiclient.TaskResponse // all fetched tasks
	filtered []apiclient.TaskResponse // after filter application
	cursor   int
	offset   int
	width    int
	height   int
	loaded   bool
	toggling bool // prevents double-toggle
	err      error
	statusMsg string

	grouped  bool // toggle: flat list vs grouped by document
	filterBar FilterBar

	client  *apiclient.Client
	vaultID string
}
```

### Display

**Flat view** (default):
```
/ status:open                                         5/12 tasks
> ☐ Fix search scoring bug         #backend  due:2026-03-28  bugs/search.md
  ☐ Add rate limiting              #backend  due:2026-03-30  ops/rate-limit.md
  ☑ Update API docs                #docs                     api/openapi.md
  ☐ Review PR feedback                       due:2026-03-25  misc/reviews.md

  enter: open  space: toggle  g: group  esc: quit
```

**Grouped view** (`g` toggle):
```
/ status:open                                         5/12 tasks
  bugs/search.md
  > ☐ Fix search scoring bug       #backend  due:2026-03-28
    ☐ Add fuzzy matching            #backend

  ops/rate-limit.md
    ☐ Add rate limiting             #backend  due:2026-03-30

  enter: open  space: toggle  g: ungroup  esc: quit
```

### Styling

Reuse exported styles from `internal/tui/pick/styles.go` (`PrimaryColor`, `MutedColor`). Colors not yet exported (accent, error, secondary) should be added to `pick/styles.go` so both the agent TUI (`tui/tasks.go`) and browse tabs can share them.

- Checkbox: `☐` (muted) / `☑` (accent color)
- Done tasks: strikethrough + muted text
- Labels: secondary color, `#label` format
- Due date: muted; red if overdue, yellow if within 7 days
- Doc path: muted, right-aligned or after task text
- Selected row: bold, `> ` prefix
- Group header: bold doc path, not selectable

### Key Bindings

| Key | Action |
|-----|--------|
| `up`/`down` | Navigate task list |
| `pgup`/`pgdown` | Scroll by page |
| `enter` | Open source document in viewer |
| `space` | Toggle task status (open/done) |
| `g` | Toggle grouped/flat view |
| `esc` | Quit browse |
| Text input | Filter bar (active when typing) |

### Sort Order

1. Open tasks before done tasks
2. Overdue tasks first (sorted by due date ascending)
3. Tasks with due dates before tasks without
4. Tasks without due dates sorted by document path

### Filter Application

Filters from the `FilterBar` map to `apiclient.TaskFilter` fields:

```go
func (t *tasksModel) buildFilter() apiclient.TaskFilter {
	r := t.filterBar.Result()
	f := apiclient.TaskFilter{}

	if v := r.Filter("status"); v != "" {
		f.Status = v // "open" or "done"
	}
	if v := r.Filter("from"); v != "" {
		f.Path = v
	}
	if v := r.Filter("due"); v != "" {
		switch v {
		case "today":
			f.DueBefore = tomorrow()
			f.DueAfter = today()
		case "week":
			f.DueBefore = endOfWeek()
			f.DueAfter = today()
		case "overdue":
			f.DueBefore = today()
		}
	}
	f.Labels = r.FilterAll("label")
	return f
}
```

The `Query` portion is applied as a local fuzzy match on task text after API filtering.

### Toggle Behavior

1. User presses `space` on a task.
2. Set `toggling = true` to prevent double-tap.
3. Call `client.ToggleTask(ctx, vaultID, taskID)`.
4. On success: update task status in-place, set `toggling = false`.
5. If `resp.Message != ""`, the task ID changed during re-ingestion — refetch the full list.

### Integration with Browser

**Tab registration** (`tabs.go`):
```go
const (
	TabSearch Tab = iota
	TabAllFiles
	TabLinks
	TabBookmarks
	TabTags
	TabTasks // new
	tabCount
)
```

**Browser model** (`browser.go`):
- Add `tasks tasksModel` field.
- Initialize in `NewModel()`: `tasks: newTasksModel(client, vaultID)`.
- Load in `Init()`: add `m.tasks.loadTasks()` to command batch (eager loading, like all other tabs).
- Route in `Update()` and `View()` for `TabTasks`.
- Add `case TabTasks:` to `documentFetchedMsg` and `audioReadyMsg` error handlers.
- In `focusActiveTab()`: blur tasks filter bar alongside all other inputs, focus it when `TabTasks` is active.

**Tab label**: `"Tasks"` — count shown in the filter bar's count line, not in the tab label (consistent with other tabs).

## Edge Cases

- **Empty task list**: Show "No tasks found." centered message.
- **No matches after filter**: Show "0/N tasks" count, empty list area.
- **Toggle during load**: `toggling` flag prevents concurrent toggles.
- **Overdue detection**: Compare `time.Parse("2006-01-02", dueDate)` against `time.Now()`. Nil due date = no color.
- **Long task text**: Truncate to available width minus label/date/path columns.
- **Group toggle preserves cursor**: When switching views, try to keep the same task selected.

## Files Changed

| File | Change |
|------|--------|
| `internal/tui/browse/filterbar.go` | **New** — FilterBar component |
| `internal/tui/browse/tasks.go` | **New** — Tasks tab model |
| `internal/tui/browse/browser.go` | Add tasks model, route TabTasks |
| `internal/tui/browse/tabs.go` | Add TabTasks constant and label |
| `internal/tui/browse/links.go` | Replace textinput + view toggle with FilterBar |
| `internal/tui/browse/finder.go` | Replace pick input with FilterBar |
| `internal/tui/browse/bookmarks.go` | Add FilterBar |
| `internal/tui/browse/search.go` | Wrap input with FilterBar |
| `internal/tui/browse/tags.go` | Wrap input with FilterBar |
| `internal/tui/pick/styles.go` | Export shared colors (AccentColor, SecondaryColor, ErrorColor) |

## Out of Scope

- Filter syntax highlighting in the text input (bubbles v2 textinput doesn't support per-character styling).
- Server-side fuzzy search for tasks (local fuzzy match is sufficient for typical vault sizes).
- Task creation from the browse TUI.
- Drag-and-drop reordering of tasks.
