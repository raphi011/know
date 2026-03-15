# Tasks

Know automatically extracts markdown checkboxes from documents during ingestion, enabling you to query, filter, and manage tasks across your entire knowledge base. The source markdown files remain the single source of truth — toggling a task modifies the original document.

## Task Format

Tasks are standard markdown checkboxes with optional metadata:

```markdown
- [ ] open task
- [x] completed task
- [ ] deploy staging #work #urgent due:2026-03-20
```

### Supported metadata

| Syntax | Description |
|--------|-------------|
| `- [ ]` / `- [x]` | Task status (open / done) |
| `#label` | Inline labels for filtering (case-insensitive, deduplicated) |
| `due:YYYY-MM-DD` | Due date for deadline-based filtering |

Labels follow the same rules as document labels: must start with a letter, can contain letters, digits, hyphens, and underscores. `#42` is not treated as a label (starts with digit).

## How It Works

1. During document ingestion, the parser extracts all checkboxes with their metadata
2. Tasks are stored in the database linked to their source document
3. Each task gets a stable identity based on its text content (a content hash)
4. Re-ingesting a document updates existing tasks rather than duplicating them
5. Toggling a task modifies the checkbox in the source markdown and re-ingests

### Content hash identity

Tasks are identified by a hash of their *cleaned text* (with `due:` dates and `#labels` stripped). This means:

- Moving a task to a different line doesn't create a duplicate
- Changing a due date or label updates the existing task record
- Only changing the actual task description creates a new task

## REST API

### List tasks

```
GET /api/tasks?vault={id}&status=open&labels=work,urgent&due_before=2026-03-20&folder=/daily/
```

Query parameters:

| Parameter | Description |
|-----------|-------------|
| `vault` | Vault ID (required) |
| `status` | `open` or `done` |
| `labels` | Comma-separated labels (matches tasks with any of these labels) |
| `due_before` | Tasks due on or before this date (YYYY-MM-DD) |
| `due_after` | Tasks due on or after this date (YYYY-MM-DD) |
| `folder` | Filter by document folder path prefix |
| `path` | Filter by exact document path |
| `limit` | Max results (default 100) |
| `offset` | Pagination offset |

### Toggle task

```
POST /api/tasks/{id}/toggle
```

Toggles the task's checkbox in the source document (`- [ ]` ↔ `- [x]`) and re-ingests the document.

## MCP Tools

### list_tasks

List tasks with filters for status, labels, due dates, and folders. Output is grouped by document path with task IDs for use with `toggle_task`.

### toggle_task

Toggle a task by ID. Modifies the source markdown document.

## CLI: `know task`

Interactive TUI for browsing and toggling tasks. Opens a fullscreen list with fuzzy filtering, keyboard navigation, and in-place toggling.

```bash
# All open tasks
know task

# Filter by labels
know task --labels work,urgent

# Show all tasks (open + done)
know task --status all

# Tasks from a specific folder
know task --path /daily/

# Tasks due within the next week
know task --due-before 2026-03-22

# Combine filters
know task --labels work --due-before 2026-03-20 --path /projects/
```

### Keybindings

| Key | Action |
|-----|--------|
| `enter` / `space` | Toggle task (check/uncheck) |
| `/` | Fuzzy filter tasks |
| `j` / `k` / arrows | Navigate |
| `Esc` / `Ctrl+C` | Quit |

### Flags

| Flag | Description |
|------|-------------|
| `--labels` | Comma-separated labels to filter by |
| `--status` | `open` (default), `done`, or `all` |
| `--due-before` | Tasks due on or before this date (YYYY-MM-DD) |
| `--due-after` | Tasks due on or after this date (YYYY-MM-DD) |
| `--path` | Document path (exact) or folder prefix (ending with `/`) |
| `--vault` | Vault name (default from `KNOW_VAULT` env) |

## Example Prompts

- "Show me all my open tasks"
- "What tasks are due this week?"
- "List open tasks from my daily notes"
- "Show tasks labeled #work"
- "Mark the PR review task as done"
- "What did I complete today?"
- "Show overdue tasks"

## Future Ideas

### Task views (kanban boards)

Task view documents could define custom views using frontmatter and headings:

```markdown
---
type: task-view
view: kanban
filters:
  labels: [tasks]
  folder: /daily/
---

# To Do

# In Progress

# Done
```

**Column mapping approaches under consideration:**

- **Label-based convention**: column headers auto-map to task labels (kebab-case). "In Progress" → matches `#in-progress`. "Done" = completed tasks. First column = default catch-all.
- **Explicit frontmatter mapping**: each column declares `match` criteria (`status`, `labels`) in YAML.
- **Convention + override**: default to label convention, allow `columns:` frontmatter to override.

### Additional metadata

- Priority levels (`!high`, `!low` or `p1`, `p2`, `p3`)
- Recurrence (`every:weekday`, `every:monday`)
- Scheduled/start dates (`scheduled:2026-03-18`)
- Assignees
