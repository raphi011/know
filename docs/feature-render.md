# Render Pipeline

The render pipeline enhances raw markdown content on read by resolving wiki-links and executing query blocks. It sits between the blob store (raw content) and the API/agent response.

## Where It Applies

| Access Method | Content Type |
|--------------|-------------|
| REST API (`GET /api/documents`) | Enhanced (rendered) |
| Agent `read_document` tool | Enhanced (rendered) |
| WebDAV | Raw (for editors) |
| NFS | Raw (for editors) |
| SFTP | Raw (for editors) |

Editors get raw content so they can edit wiki-link syntax directly. API and agent consumers get the rendered version for display and reasoning.

## Wiki-Link Resolution

Wiki-links (`[[target]]` or `[[target|alias]]`) are resolved to standard markdown links using the DB's wiki-link resolution data.

### Before (raw)
```markdown
See [[Meeting Notes]] for details.
Check [[projects/alpha|Alpha Project]] too.
```

### After (rendered)
```markdown
See [Meeting Notes](/meetings/meeting-notes.md) for details.
Check [Alpha Project](/projects/alpha.md) too.
```

- **Resolved links**: replaced with `[display text](path)`
- **Dangling links** (target not found): left as `[[target]]`
- **Alias syntax**: `[[target|alias]]` uses the alias as display text
- **No alias**: uses the resolved document's title, falling back to the raw target

## Query Blocks

Fenced code blocks with the `know` language tag execute queries against the vault's file index and render results inline. Queries must start with a format keyword: `LIST`, `TABLE`, or `TASK`.

### LIST Format

````markdown
```know
LIST
FROM /daily
WHERE labels CONTAIN "meeting"
SORT updated_at DESC
LIMIT 5
```
````

Renders as:
```markdown
- [Standup 2024-03-18](/daily/standup-2024-03-18.md)
- [Retro Q1](/daily/retro-q1.md)
```

LIST with an extra field:
````markdown
```know
LIST labels
FROM /projects
```
````

LIST WITHOUT ID (plain text, no links):
````markdown
```know
LIST WITHOUT ID title
FROM /projects
```
````

### TABLE Format

````markdown
```know
TABLE title, labels AS "Tags", mime_type
FROM /projects
LIMIT 10
```
````

Renders as a markdown table. A "File" column with `[title](path)` is auto-prepended unless `WITHOUT ID` is used.

TABLE WITHOUT ID (no auto file column):
````markdown
```know
TABLE WITHOUT ID title AS "Name", labels AS "Tags"
FROM /projects
```
````

### TASK Format

````markdown
```know
TASK
WHERE status = "open"
SORT due_date ASC
LIMIT 20
```
````

Renders as markdown checkboxes:
```markdown
- [ ] Review PR for auth module — */daily/2025-03-24.md*
- [x] Update deployment docs — */ops/runbook.md*
- [ ] Fix search ranking (due: 2025-03-28) — */bugs/search.md*
```

TASK with filters:
````markdown
```know
TASK
FROM /daily
WHERE labels CONTAIN "urgent"
WHERE status = "open"
```
````

### Supported Query Syntax

All queries must start with a format keyword:

- `LIST [field]` — bullet list of links, optional extra field per item
- `TABLE [field1, field2 AS "Alias", ...]` — markdown table with columns
- `TASK` — checkbox list of tasks

Modifiers:
- `WITHOUT ID` — suppress the auto file link (LIST: plain text; TABLE: no File column; TASK: no doc path)
- `field AS "Alias"` — custom column header (TABLE only)

Clauses (order-independent, after format keyword):
- `FROM /folder` — filter by folder prefix
- `WHERE labels CONTAIN "value"` — filter by label
- `WHERE doc_type = "value"` — filter by document type
- `WHERE mime_type = "value"` — filter by MIME type
- `WHERE status = "open"` — filter tasks by status (TASK only)
- `WHERE due_before = "YYYY-MM-DD"` — tasks due before date (TASK only)
- `WHERE due_after = "YYYY-MM-DD"` — tasks due after date (TASK only)
- `SORT field ASC|DESC` — sort by `updated_at`, `created_at`, `due_date`, or `path`
- `LIMIT n` — max results (default: 50)

Both ` ``` ` and `~~~` fence styles are supported.

## Example Prompts

```
"Read my daily notes index"
"Show me the document at /projects/overview.md"
"What's in my meeting notes from this week?"
```

The render pipeline runs transparently — agents and API consumers see enhanced content without needing to know about wiki-links or query blocks.
