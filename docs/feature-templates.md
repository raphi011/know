# Templates

Templates are reusable document structures stored as regular documents in a configurable folder (default: `/templates/`). They are excluded from search indexing and chunking but remain browsable and editable via all standard interfaces (WebDAV, MCP, CLI, REST API).

## How It Works

- Templates are just markdown documents stored under the vault's template folder
- They are **not chunked or indexed** — they won't appear in search results
- The agent's system prompt lists available templates so it knows what's available
- When asked to use a template, the agent reads it with `read_document` and structures its output accordingly

## Creating Templates

### Via CLI

```bash
# Copy a template file into the vault
know cp ./my-template.md /templates/research.md --vault default
```

### Via MCP

```
Create a new document at /templates/meeting-notes.md with the following content:
---
title: "Meeting Notes"
labels:
  - template
---
# Meeting: {{title}}
Date: {{date}}

## Attendees

## Agenda

## Discussion

## Action Items
```

### Via WebDAV

Open `/templates/` in any WebDAV-compatible editor and create/edit templates directly.

## Built-in Variables

Templates support placeholder variables that are substituted programmatically (e.g., when creating daily notes without an agent):

| Variable | Description | Example |
|----------|-------------|---------|
| `{{date}}` | Current date | `2026-03-14` |
| `{{datetime}}` | Current date and time | `2026-03-14 15:04` |
| `{{title}}` | Provided title | `My Note` |
| `{{vault}}` | Vault name | `default` |

Variables are replaced using simple string substitution. Unknown variables are left as-is.

**Note:** Variable substitution only happens programmatically (e.g., daily note creation via `know note`). When the agent uses a template, it reads the raw content and fills in sections intelligently rather than relying on variable substitution.

## Daily Note Integration

Daily notes (`know note`) automatically use a template if one exists at `/templates/daily-note.md`. Variables like `{{date}}` are substituted when the note is created. If no template exists, the default format is used:

```markdown
---
title: "2026-03-14"
labels:
  - daily-note
---
# 2026-03-14
```

### Custom daily note template example

Create `/templates/daily-note.md`:

```markdown
---
title: "{{date}}"
labels:
  - daily-note
---
# {{date}}

## Morning intentions

## Tasks

- [ ]

## Notes

## Evening reflections
```

## Configuration

The template folder path is configurable per-vault via the `template_path` field in vault settings (default: `/templates`). This can be set when creating or updating a vault through the REST API.

For the `know note` command, the template folder can also be set via:
- `--template-folder` flag
- `KNOW_TEMPLATE_FOLDER` environment variable

## Example Prompts

### Agent uses a template for output

> "Research the pros and cons of SQLite vs PostgreSQL for embedded use cases. Use the research template."

The agent reads `/templates/research.md`, understands its structure, and writes its response following that format.

### Creating a note with a template

> "Create a new meeting note for today's standup with Alice and Bob."

If a `/templates/meeting-notes.md` template exists, the agent can read it and create a structured meeting note.

### Daily note via CLI

```bash
# Creates today's note using /templates/daily-note.md if it exists
know note "Had a productive morning call with the team"
```

### Listing templates

> "What templates do I have?"

The agent sees the template list in its system prompt and can list them directly.
