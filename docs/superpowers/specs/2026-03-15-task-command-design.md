# Design: `know task` — Interactive Task Command

## Overview

A new `know task` CLI command that opens an interactive Bubbletea v2 TUI for browsing and toggling tasks. Supports label, status, date, and path filtering via CLI flags.

## CLI Interface

```
know task [flags]

Flags:
  --labels strings       Filter by labels (comma-separated)
  --status string        Filter by status: open, done, all (default "open")
  --due-before string    Only tasks due on or before this date (YYYY-MM-DD)
  --due-after string     Only tasks due on or after this date (YYYY-MM-DD)
  --path string          Filter by document path or folder (see Path Resolution)
  --vault string         Vault name (default from env/config)
```

Examples:
```bash
know task                           # all open tasks
know task --labels work,urgent      # open tasks with work or urgent label
know task --status all              # open + done
know task --path /daily/            # tasks from docs under /daily/
know task --due-before 2026-03-20   # due within 5 days
```

### Status flag behavior

- `--status open` (default) → sends `status=open` to API
- `--status done` → sends `status=done` to API
- `--status all` → omits `status` param (API returns all tasks when unset)

## Path Resolution

Path resolution is done **client-side** in the CLI:

- Path ends with `/` → sent as `folder` query param (prefix match)
- Otherwise → sent as `path` query param (exact document match)

No API changes needed — the existing `path` and `folder` params are used directly.

## TUI Layout

```
Tasks (12 open)

  ☐ Write weekly report            #work  due:2026-03-17
  ☑ Review PR #132                 #work
> ☐ Fix login bug                  #work  #urgent
  ☐ Buy groceries
  ☐ Call dentist                          due:2026-03-20

  1/5
  / filter • esc quit
```

### Rendering per item

- Checkbox: `☐` (open) / `☑` (done), colored (dimmed for done)
- Task text: main content
- Labels: after text, subtle color
- Due date: after labels, colored red if overdue

### Keybindings

- `enter` or `space` — toggle selected task (API call, update item in-place)
- `Esc` / `Ctrl+C` — quit
- `/` — enter filter mode (built-in fuzzy search over task text)
- `j`/`k`, arrows — navigate
- `q` — unbound (no action); default list quit binding disabled

## Architecture

### Component: `bubbles/v2/list`

Uses the bubbles v2 list component with a custom `ItemDelegate` for rendering. The list provides cursor navigation, fuzzy filtering, pagination, and in-place item updates out of the box.

### Data loading

All matching tasks are fetched upfront in a single API call with `limit=1000`. This is sufficient for the expected scale of personal knowledge bases.

### Toggle response handling

The toggle API (`POST /api/tasks/{id}/toggle`) returns two possible response shapes:

1. **Task object** (has `id` field) — task was toggled, ID unchanged. Deserialize into `TaskResponse`, update item in-place via `m.SetItem(globalIdx, updatedItem)`.
2. **Message object** (has `message` field, no `id`) — task was toggled but ID changed due to re-ingestion. Refetch the full task list and rebuild the list items, preserving the cursor index.

Deserialization strategy: unmarshal into a struct with all fields optional; check if `ID` is non-empty to distinguish.

### Data Flow

```
CLI flags → apiclient.ListTasks() → GET /api/tasks?... → response
                                                              ↓
                                                    Convert to list.Item[]
                                                              ↓
                                                    list.New(items, delegate)
                                                              ↓
                                                    tea.NewProgram(model).Run()

User presses enter/space on item:
    delegate.Update() → apiclient.ToggleTask(id) → POST /api/tasks/{id}/toggle
                                                              ↓
                                                    response with updated task
                                                              ↓
                                                    m.SetItem(globalIdx, updatedItem)
                                                    OR refetch list if ID changed
```

### New Files

- `cmd/know/cmd_task.go` — Cobra command, flag parsing, fetch tasks, launch TUI
- `internal/tui/tasks.go` — Bubbletea model wrapping `list.Model`, custom item delegate, toggle logic
- `internal/apiclient/tasks.go` — `ListTasks(ctx, filter)` and `ToggleTask(ctx, id)` methods

### Modified Files

- `cmd/know/main.go` — register `taskCmd`

## Error Handling

- **API unreachable**: TUI doesn't start, CLI prints error and exits
- **Toggle fails**: Show as status message in the list title bar (built-in `NewStatusMessage`), keep cursor position, don't change checkbox state
- **Empty result set**: Show list with "No tasks found" empty state (built-in in `list.Model`)
- **Toggle returns changed ID**: Refetch full list, preserve cursor index (see Toggle response handling above)
