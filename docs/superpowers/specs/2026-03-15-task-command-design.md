# Design: `know task` — Interactive Task Command

## Overview

A new `know task` CLI command that opens an interactive Bubbletea v2 TUI for browsing and toggling tasks. Supports label, status, date, and path filtering via CLI flags.

## CLI Interface

```
know task [flags]

Flags:
  --labels strings     Filter by labels (comma-separated)
  --status string      Filter by status: open, done, all (default "open")
  --due-before date    Only tasks due on or before this date
  --due-after date     Only tasks due on or after this date
  --path string        Filter by document path (exact) or folder (prefix)
  --vault string       Vault name (default from env/config)
```

Examples:
```bash
know task                           # all open tasks
know task --labels work,urgent      # open tasks with work or urgent label
know task --status all              # open + done
know task --path /daily/            # tasks from docs under /daily/
know task --due-before 2026-03-20   # due within 5 days
```

## Path Resolution

The `--path` flag accepts any path. The server determines whether it's a document or folder:

1. CLI sends `path` to `GET /api/tasks`
2. API handler checks if path matches an existing document → uses `path` filter (exact)
3. If no document match → uses `folder` filter (prefix match)

This keeps the CLI simple — one flag, server decides. Requires a small API change: accept a generic `path` param and resolve it server-side, instead of separate `path`/`folder` params.

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
- `q` — disabled as quit key

## Architecture

### Component: `bubbles/v2/list`

Uses the bubbles v2 list component with a custom `ItemDelegate` for rendering. The list provides cursor navigation, fuzzy filtering, pagination, and in-place item updates out of the box.

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
```

### New Files

- `cmd/know/cmd_task.go` — Cobra command, flag parsing, fetch tasks, launch TUI
- `internal/tui/tasks.go` — Bubbletea model wrapping `list.Model`, custom item delegate, toggle logic
- `internal/apiclient/tasks.go` — `ListTasks(ctx, filter)` and `ToggleTask(ctx, id)` methods

### Modified Files

- `internal/api/tasks.go` — merge `path`/`folder` param into single `path` with auto-resolution
- `cmd/know/main.go` — register `taskCmd`

## Error Handling

- **API unreachable**: TUI doesn't start, CLI prints error and exits
- **Toggle fails**: Show as status message in the list title bar (built-in `NewStatusMessage`), keep cursor position, don't change checkbox state
- **Empty result set**: Show list with "No tasks found" empty state (built-in in `list.Model`)
- **Toggle returns changed ID**: Task re-ingestion can change IDs. On toggle response, if the API returns the updated task, update the item. If it returns the "ID may have changed" message, refetch the full list.
