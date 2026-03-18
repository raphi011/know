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

Fenced code blocks with the `know` language tag execute queries against the vault's file index and render results inline.

### List Format (default)

````markdown
```know
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

### Table Format

````markdown
```know
FROM /projects
SHOW title, path, labels
FORMAT table
LIMIT 10
```
````

Renders as a markdown table with the specified columns.

### Supported Query Syntax

- `FROM /folder` — filter by folder prefix
- `WHERE labels CONTAIN "value"` — filter by label
- `WHERE doc_type = "value"` — filter by document type
- `WHERE mime_type = "value"` — filter by MIME type
- `SORT field ASC|DESC` — sort by `updated_at`, `created_at`, or `path`
- `LIMIT n` — max results (default varies)
- `SHOW field1, field2, ...` — columns for list/table output
- `FORMAT list|table` — output format (default: list)

Both ` ``` ` and `~~~` fence styles are supported.

## Example Prompts

```
"Read my daily notes index"
"Show me the document at /projects/overview.md"
"What's in my meeting notes from this week?"
```

The render pipeline runs transparently — agents and API consumers see enhanced content without needing to know about wiki-links or query blocks.
