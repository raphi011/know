# Filter Bar + Tasks Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a reusable FilterBar component to the browse TUI and a new Tasks tab with inline filter support, then roll out the FilterBar to all existing tabs.

**Architecture:** The FilterBar is a pure input parser wrapping `textinput.Model` that splits `key:value` filter tokens from free text. Each tab declares supported filter keys; unsupported keys become search text. The Tasks tab follows the same patterns as the existing links/bookmarks tabs (cursor navigation, async messages, focus/blur).

**Tech Stack:** Go, Bubbletea v2 (`charm.land/bubbletea/v2`), Bubbles v2 (`charm.land/bubbles/v2`), Lipgloss v2 (`charm.land/lipgloss/v2`)

**Spec:** `docs/superpowers/specs/2026-03-25-filter-bar-tasks-tab-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/tui/browse/filterbar.go` | Create | FilterBar component: wraps textinput, parses `key:value` filters |
| `internal/tui/browse/filterbar_test.go` | Create | FilterBar parsing tests |
| `internal/tui/browse/tasks.go` | Create | Tasks tab: model, update, view, toggle, grouped/flat |
| `internal/tui/browse/tabs.go` | Modify | Add `TabTasks` constant, update `renderTabs` signature |
| `internal/tui/browse/browser.go` | Modify | Integrate tasks model: Init, Update, View, focus, sizing |
| `internal/tui/pick/styles.go` | Modify | Export shared colors (AccentColor, ErrorColor) |
| `internal/tui/browse/links.go` | Modify | Replace textinput + `v` toggle with FilterBar |
| `internal/tui/browse/finder.go` | Modify | Replace pick input with FilterBar for filter extraction |
| `internal/tui/browse/bookmarks.go` | Modify | Add FilterBar with `label` support |
| `internal/tui/browse/search.go` | Modify | Wrap input with FilterBar for `label` extraction |
| `internal/tui/browse/tags.go` | Modify | Wrap input with FilterBar (no supported keys) |

---

### Task 1: Export Shared Colors

**Files:**
- Modify: `internal/tui/pick/styles.go`
- Modify: `internal/tui/tasks.go` (consume exported colors)

- [ ] **Step 1: Read current styles**

Read `internal/tui/pick/styles.go` and `internal/tui/tasks.go` to identify which colors are defined where and what needs exporting.

- [ ] **Step 2: Export colors from pick/styles.go**

Add exported color constants to `internal/tui/pick/styles.go`. Read the actual color values from `internal/tui/styles.go` (the canonical source — colors are defined there as unexported vars) and export them from `pick/styles.go`. Do NOT hardcode color values — read and use the exact values from `internal/tui/styles.go`.

- [ ] **Step 3: Update tasks.go to use exported colors**

Replace the unexported color vars in `internal/tui/tasks.go` with references to `pick.AccentColor`, `pick.ErrorColor`, etc. Remove the duplicated color definitions.

- [ ] **Step 4: Build and verify**

Run: `just build`
Expected: Clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/pick/styles.go internal/tui/tasks.go
git commit -m "refactor: export shared TUI colors from pick/styles"
```

---

### Task 2: FilterBar Component

**Files:**
- Create: `internal/tui/browse/filterbar.go`
- Create: `internal/tui/browse/filterbar_test.go`

- [ ] **Step 1: Write FilterBar parsing tests**

Create `internal/tui/browse/filterbar_test.go`. Test the `parseFilterInput` function (pure logic, no TUI):

```go
func TestParseFilterInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		supported []string
		wantQuery string
		wantFilters map[string][]string
	}{
		{
			name:      "plain text only",
			input:     "fix bug",
			supported: []string{"status", "label"},
			wantQuery: "fix bug",
			wantFilters: map[string][]string{},
		},
		{
			name:      "single filter",
			input:     "status:open",
			supported: []string{"status", "label"},
			wantQuery: "",
			wantFilters: map[string][]string{"status": {"open"}},
		},
		{
			name:      "filter and text mixed",
			input:     "fix label:go bug status:open",
			supported: []string{"status", "label"},
			wantQuery: "fix bug",
			wantFilters: map[string][]string{"label": {"go"}, "status": {"open"}},
		},
		{
			name:      "multi-value filter",
			input:     "label:go label:rust",
			supported: []string{"label"},
			wantQuery: "",
			wantFilters: map[string][]string{"label": {"go", "rust"}},
		},
		{
			name:      "unsupported key becomes query text",
			input:     "status:open host:github.com",
			supported: []string{"status"},
			wantQuery: "host:github.com",
			wantFilters: map[string][]string{"status": {"open"}},
		},
		{
			name:      "empty input",
			input:     "",
			supported: []string{"status"},
			wantQuery: "",
			wantFilters: map[string][]string{},
		},
		{
			name:      "colon in value",
			input:     "from:/docs/notes",
			supported: []string{"from"},
			wantQuery: "",
			wantFilters: map[string][]string{"from": {"/docs/notes"}},
		},
		{
			name:      "key with no value ignored as query text",
			input:     "status: something",
			supported: []string{"status"},
			wantQuery: "status: something",
			wantFilters: map[string][]string{},
		},
	}
	// ... test loop with reflect.DeepEqual or maps.Equal
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `just test`
Expected: Compilation error — `parseFilterInput` not defined.

- [ ] **Step 3: Implement FilterBar**

Create `internal/tui/browse/filterbar.go`:

```go
package browse

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/raphi011/know/internal/tui/pick"
)

// FilterBarConfig declares which filter keys a tab supports.
type FilterBarConfig struct {
	SupportedKeys []string
	Placeholder   string
}

// FilterResult holds the parsed output of the filter input.
type FilterResult struct {
	Query   string
	Filters map[string][]string
}

// Filter returns the first value for a key, or empty string.
func (r FilterResult) Filter(key string) string {
	if vals, ok := r.Filters[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// FilterAll returns all values for a key.
func (r FilterResult) FilterAll(key string) []string {
	return r.Filters[key]
}

// FilterBar wraps textinput.Model and parses key:value filter tokens.
type FilterBar struct {
	input     textinput.Model
	config    FilterBarConfig
	result    FilterResult
	supported map[string]bool // derived from config for O(1) lookup
}

// NewFilterBar creates a filter bar with the given config.
func NewFilterBar(config FilterBarConfig) FilterBar {
	ti := textinput.New()
	ti.Placeholder = config.Placeholder
	ti.CharLimit = 256
	ti.Prompt = "/ "
	styles := ti.Styles()
	styles.Focused.Prompt = pick.PromptStyle
	styles.Cursor.Blink = false
	ti.SetStyles(styles)

	supported := make(map[string]bool, len(config.SupportedKeys))
	for _, k := range config.SupportedKeys {
		supported[k] = true
	}

	return FilterBar{
		input:     ti,
		config:    config,
		supported: supported,
		result:    FilterResult{Filters: make(map[string][]string)},
	}
}

// Update handles input messages and re-parses on change.
func (f FilterBar) Update(msg tea.Msg) (FilterBar, tea.Cmd) {
	prev := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != prev {
		f.result = parseFilterInput(f.input.Value(), f.supported)
	}
	return f, cmd
}

// View renders the text input.
func (f FilterBar) View() string {
	return f.input.View()
}

// Result returns the current parsed filter state.
func (f FilterBar) Result() FilterResult {
	return f.result
}

// Focus focuses the text input.
func (f FilterBar) Focus() (FilterBar, tea.Cmd) {
	cmd := f.input.Focus()
	return f, cmd
}

// Blur blurs the text input.
func (f FilterBar) Blur() FilterBar {
	f.input.Blur()
	return f
}

// Value returns the raw input text.
func (f FilterBar) Value() string {
	return f.input.Value()
}

// SetValue sets the raw input text and re-parses.
func (f FilterBar) SetValue(v string) FilterBar {
	f.input.SetValue(v)
	f.result = parseFilterInput(v, f.supported)
	return f
}

// parseFilterInput splits input into recognized key:value filters and remaining query text.
func parseFilterInput(input string, supported map[string]bool) FilterResult {
	result := FilterResult{Filters: make(map[string][]string)}
	if input == "" {
		return result
	}

	var queryParts []string
	for _, token := range strings.Fields(input) {
		key, value, hasColon := strings.Cut(token, ":")
		if hasColon && value != "" && supported[key] {
			result.Filters[key] = append(result.Filters[key], value)
		} else {
			queryParts = append(queryParts, token)
		}
	}
	result.Query = strings.Join(queryParts, " ")
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse/filterbar.go internal/tui/browse/filterbar_test.go
git commit -m "feat(tui): add FilterBar component with key:value parsing"
```

---

### Task 3: Tasks Tab — Model + Data Loading

**Files:**
- Create: `internal/tui/browse/tasks.go`
- Modify: `internal/tui/browse/tabs.go`

- [ ] **Step 1: Add TabTasks to tabs.go**

Read `internal/tui/browse/tabs.go`. Add `TabTasks` before `tabCount` in the Tab enum. Update `renderTabs` to accept a `taskCount int` parameter and add `TabTasks` to the tabs slice with label `"Tasks"`.

- [ ] **Step 2: Create tasks.go with model and data loading**

Create `internal/tui/browse/tasks.go` with:
- Message types: `tasksLoadedMsg`, `taskToggledMsg`
- `tasksModel` struct with fields: `allTasks`, `filtered`, `cursor`, `offset`, `width`, `height`, `loaded`, `toggling`, `grouped`, `statusErr`, `statusOK`, `filterBar`, `client`, `vaultID`
- `newTasksModel(client, vaultID)` constructor
- `loadTasks() tea.Cmd` — calls `client.ListTasks` with filter from filterBar
- `buildFilter() apiclient.TaskFilter` — maps FilterResult to TaskFilter (status, label, due, from)
- `applyFilter()` — applies fuzzy text match from FilterResult.Query on task text + doc path
- `ensureCursorVisible()`, `visibleRows()`, `selectedTask()` helpers

FilterBar config for tasks:
```go
NewFilterBar(FilterBarConfig{
	SupportedKeys: []string{"status", "label", "due", "from"},
	Placeholder:   "Filter tasks... (status:open label:go due:overdue)",
})
```

The `buildFilter` method maps:
- `status` → `TaskFilter.Status`
- `label` → `TaskFilter.Labels` (via `FilterAll`)
- `from` → `TaskFilter.Path`
- `due:today` → DueAfter=today, DueBefore=tomorrow
- `due:week` → DueAfter=today, DueBefore=end of week
- `due:overdue` → DueBefore=today

- [ ] **Step 3: Build and verify**

Run: `just build`
Expected: Clean build. The tasks model isn't wired into browser.go yet, so just verify compilation.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/browse/tasks.go internal/tui/browse/tabs.go
git commit -m "feat(tui): add tasks tab model with data loading and filtering"
```

---

### Task 4: Tasks Tab — Update + View + Key Bindings

**Files:**
- Modify: `internal/tui/browse/tasks.go`

- [ ] **Step 1: Implement Update method**

Add `Update(msg tea.Msg) (tasksModel, tea.Cmd)` to tasksModel. Handle:
- `tasksLoadedMsg` — set `loaded=true`, store tasks, call `applyFilter()`
- `taskToggledMsg` — clear `toggling`, handle error or reload tasks
- `tea.KeyPressMsg`:
  - `up`/`down` — cursor navigation
  - `pgup`/`pgdown` — page navigation
  - `space` — toggle task status (if not already toggling)
  - `enter` — return `fileSelectedMsg` with the task's document path (to open in viewer)
  - `g` — toggle `grouped` bool, call `applyFilter()`
- Delegate remaining messages to filterBar. If filterBar value changed, call `applyFilter()`.

When filter changes, also check if we need to re-fetch from API (if structured filters like `status:`, `label:`, `due:`, `from:` changed) vs just re-filter locally (if only query text changed). For simplicity in v1: always re-fetch from API when any filter token changes, local fuzzy match for query text only.

- [ ] **Step 2: Implement View method**

Add `View() string` to tasksModel. Render:
1. FilterBar view
2. Count line: `"  N/M tasks"` (filtered/total)
3. Task rows (flat or grouped based on `grouped` flag):
   - **Flat**: `> ☐ Task text  #label  due:YYYY-MM-DD  docname.md`
   - **Grouped**: Group header (bold doc path), then indented tasks
4. Footer: `"  space: toggle  enter: open  g: group  esc: quit"` (or error/success message)

Use `pick.NormalStyle`/`pick.SelectedStyle` for row styling. Use exported colors from `pick/styles.go` for checkbox, due date, labels.

Sort order: open before done, overdue first, then by due date ascending, tasks without due date last, then by doc path.

- [ ] **Step 3: Build and verify**

Run: `just build`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/browse/tasks.go
git commit -m "feat(tui): add tasks tab Update/View with navigation and toggle"
```

---

### Task 5: Browser Integration

**Files:**
- Modify: `internal/tui/browse/browser.go`

- [ ] **Step 1: Read browser.go**

Read `internal/tui/browse/browser.go` to identify exact insertion points for:
- Model struct field
- NewModel constructor
- Init command batch
- WindowSizeMsg handler
- Async message routing (tasksLoadedMsg, taskToggledMsg)
- Active tab routing in Update
- View tab switch
- focusActiveTab blur/focus
- renderTabs call

- [ ] **Step 2: Add tasks field to Model struct**

Add `tasks tasksModel` field after the existing tab model fields.

- [ ] **Step 3: Initialize in NewModel**

Add `tasks: newTasksModel(client, vaultID)` to the Model literal in `NewModel()`.

- [ ] **Step 4: Wire Init**

Add `m.tasks.loadTasks()` to the Init command batch. Add `case TabTasks:` to the focus switch that runs during Init.

- [ ] **Step 5: Wire WindowSizeMsg**

In the `tea.WindowSizeMsg` handler, set `m.tasks.width` and `m.tasks.height` to the same content dimensions used by other tabs.

- [ ] **Step 6: Wire async message routing**

Add `case tasksLoadedMsg:` and `case taskToggledMsg:` to the message type switch, delegating to `m.tasks.Update(msg)`.

- [ ] **Step 6b: Wire error handler switches**

Add `case TabTasks:` to the `documentFetchedMsg` and `audioReadyMsg` error handler switches, setting `m.tasks.statusErr` on error (same pattern as other tabs).

- [ ] **Step 7: Wire active tab routing**

Add `case TabTasks:` to the active tab switch in Update, delegating to `m.tasks.Update(msg)`.

When tasks.Update returns a `fileSelectedMsg` (from pressing Enter on a task), it should flow through the existing document opening logic.

- [ ] **Step 8: Wire View**

Add `case TabTasks:` to the View tab switch, rendering `m.tasks.View()`.

Update the `renderTabs` call to pass `len(m.tasks.allTasks)` as the task count.

- [ ] **Step 9: Wire focusActiveTab**

Add `m.tasks.filterBar = m.tasks.filterBar.Blur()` to the blur section. Add `case TabTasks:` that calls `m.tasks.filterBar.Focus()` and returns the focus command.

- [ ] **Step 10: Build and test manually**

Run: `just build && just run browse`
Expected: Tasks tab appears, can switch to it, tasks load and display.

- [ ] **Step 11: Commit**

```bash
git add internal/tui/browse/browser.go
git commit -m "feat(tui): integrate tasks tab into browser"
```

---

### Task 6: Tasks Tab — Grouped View

**Files:**
- Modify: `internal/tui/browse/tasks.go`

- [ ] **Step 1: Implement grouped rendering**

Add a `viewGrouped` helper to tasksModel that:
1. Groups `filtered` tasks by `DocumentPath` (preserving sort order within groups)
2. Renders group headers as bold doc paths (non-selectable)
3. Renders tasks indented under their group header
4. Tracks a mapping from visual row index to task index for cursor navigation

The cursor should skip group header rows (they're display-only). Use a `rowMap []int` where `rowMap[visualRow] = taskIndex` (-1 for headers).

- [ ] **Step 2: Update View to use grouped rendering**

In `View()`, check `t.grouped` and call either the flat or grouped renderer.

- [ ] **Step 3: Update cursor navigation for grouped view**

In the `up`/`down` key handlers, skip non-selectable rows (group headers) by advancing past them.

- [ ] **Step 4: Build and test manually**

Run: `just build && just run browse`
Expected: Press `g` in tasks tab to toggle grouped view. Cursor skips headers.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse/tasks.go
git commit -m "feat(tui): add grouped view toggle to tasks tab"
```

---

### Task 7: Roll Out FilterBar to Links Tab

**Files:**
- Modify: `internal/tui/browse/links.go`

- [ ] **Step 1: Read links.go**

Read the full file to understand the current textinput usage and the `v` key inbox/archived toggle.

- [ ] **Step 2: Replace textinput with FilterBar**

Replace the `input textinput.Model` field with `filterBar FilterBar`. Initialize with:
```go
NewFilterBar(FilterBarConfig{
	SupportedKeys: []string{"label", "host", "status"},
	Placeholder:   "Filter links... (host:github.com status:archived)",
})
```

- [ ] **Step 3: Update filtering logic**

Replace the current `showArchived` bool + `v` toggle with FilterBar-driven filtering:
- `status:archived` → show only archived links
- `status:inbox` → show only non-archived links
- Default (no status filter) → show inbox (preserve existing default behavior)
- `host:` → filter by source URL hostname
- `label:` → filter by label
- Free text → fuzzy match on title + URL (existing behavior)

Keep `v` as a shortcut that toggles `status:archived` in the filter bar input via `SetValue`.

- [ ] **Step 4: Update focus/blur references in browser.go**

Update `browser.go`'s `focusActiveTab()` to blur/focus `m.links.filterBar` instead of `m.links.input`.

- [ ] **Step 5: Build and test manually**

Run: `just build && just run browse`
Expected: Links tab works with filter bar. `v` key pre-fills `status:archived`. `host:github.com` filters by hostname.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/browse/links.go internal/tui/browse/browser.go
git commit -m "feat(tui): replace links tab input with FilterBar"
```

---

### Task 8: Roll Out FilterBar to All Files Tab

**Files:**
- Modify: `internal/tui/browse/finder.go`

- [ ] **Step 1: Read finder.go**

Read the full file to understand how `pick.Model` is used for fuzzy matching.

- [ ] **Step 2: Add FilterBar alongside pick.Model**

The finder tab uses `pick.Model` which has its own `textinput.Model` (public field `Input`). The approach: intercept `pick.Model.Input.Value()` after each update, run it through `parseFilterInput` to extract filter tokens, then client-side filter the file list before pick's fuzzy matcher runs. This avoids modifying `pick.Model` internals.

Concretely: add a `filterSupported map[string]bool` field to `finderModel`. In Update, after pick processes the input, call `parseFilterInput(m.pick.Input.Value(), m.filterSupported)` to get filters + query. Apply `from:` as path prefix filter and `label:`/`type:` as client-side filters on the pre-loaded file list. Pass only the query portion to pick's fuzzy matching via `pick.SetFilter(query)` (or by setting the items to the pre-filtered list).

- [ ] **Step 3: Apply structured filters**

Extract `label:` and `type:` and `from:` filters. For `label:` and `type:`, these would require re-fetching from the API with filter params (currently `ListFiles` doesn't support label filtering — check if `ListFilesByLabels` can be used instead). For `from:`, filter the pre-loaded file list by path prefix.

If the API doesn't support the needed filters, apply them client-side on the pre-loaded file list.

- [ ] **Step 4: Build and test manually**

Run: `just build && just run browse`
Expected: All Files tab accepts filter tokens, unsupported ones become search text.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/browse/finder.go
git commit -m "feat(tui): add FilterBar to all files tab"
```

---

### Task 9: Roll Out FilterBar to Remaining Tabs

**Files:**
- Modify: `internal/tui/browse/bookmarks.go`
- Modify: `internal/tui/browse/search.go`
- Modify: `internal/tui/browse/tags.go`
- Modify: `internal/tui/browse/browser.go`

- [ ] **Step 1: Add FilterBar to bookmarks tab**

Bookmarks currently has no text input. Add a FilterBar with `label` support. Use it for fuzzy matching on bookmark title/path + label filtering. Add cursor-based navigation if not already present. Also add `case TabBookmarks:` to `focusActiveTab()` in browser.go (currently falls through to `default: return nil`).

- [ ] **Step 2: Add FilterBar to search tab**

Search already has a debounced textinput. Replace it with FilterBar. Pass `FilterResult.Query` to the search API. Extract `label:` filter — if the search API supports label filtering, pass it; otherwise filter results client-side.

- [ ] **Step 3: Add FilterBar to tags tab**

Tags uses dual `pick.Model` pickers. Add a FilterBar with no supported keys (all input is fuzzy match). This preserves current behavior. The FilterBar replaces the tag picker's textinput.

- [ ] **Step 4: Update browser.go focus/blur for all modified tabs**

Update `focusActiveTab()` to blur/focus the new FilterBar instances in bookmarks, search, and tags tabs.

- [ ] **Step 5: Build and test all tabs**

Run: `just build && just run browse`
Expected: All tabs work with FilterBar. Unsupported keys in any tab become search text.

- [ ] **Step 6: Run full test suite**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/browse/bookmarks.go internal/tui/browse/search.go internal/tui/browse/tags.go internal/tui/browse/browser.go
git commit -m "feat(tui): roll out FilterBar to bookmarks, search, and tags tabs"
```

---

### Task 10: Final Polish + Documentation

**Files:**
- Modify: `docs/feature-tasks.md`

- [ ] **Step 1: Update feature docs**

Add a section to `docs/feature-tasks.md` documenting the browse tasks tab:
- How to access: `know browse` → Tasks tab
- Filter syntax: `status:open`, `label:go`, `due:overdue`, `from:/path`
- Key bindings: space to toggle, enter to open, g to group
- Example prompts for the agent

- [ ] **Step 2: Run full test suite**

Run: `just test`
Expected: All tests pass.

- [ ] **Step 3: Build and manual smoke test**

Run: `just build && just run browse`
Verify: All tabs work, tasks tab loads/filters/toggles/groups correctly.

- [ ] **Step 4: Commit**

```bash
git add docs/feature-tasks.md
git commit -m "docs: add tasks tab documentation with filter syntax"
```
