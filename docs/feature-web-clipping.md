# Web Clipping

Know can fetch web pages, convert them to clean markdown via [Jina Reader](https://jina.ai/reader/), and optionally save them to your vault. Saved pages go through the full ingestion pipeline (chunking, embedding, wiki-link resolution) just like any other document.

## Configuration

```bash
# Optional — enables higher rate limits. Free tier works without a key.
KNOW_JINA_API_KEY=jina_...
```

## How It Works

1. The Jina Reader API fetches the target URL and converts it to clean markdown
2. In save mode, frontmatter is prepended with title, source URL, fetch timestamp, and a `web-clip` label
3. The document is saved to the vault's web clip folder (default: `/web/`)
4. The ingestion pipeline processes it like any other document

### Web clip folder

The default save path is `/web/<slugified-title>.md`. You can customize the folder per vault via the `web_clip_path` vault setting, or override the full path per request.

### Frontmatter

Saved pages include YAML frontmatter:

```yaml
---
title: "Page Title"
source: "https://example.com/article"
canonical_url: "https://example.com/article"  # if different from source
description: "Page meta description"
fetched_at: "2026-03-18T09:00:00Z"
labels:
  - web-clip
---
```

## CLI

```bash
# Fetch and save to default web clip folder
know fetch https://example.com/article

# Save to a custom path
know fetch https://example.com/article --path /articles/custom.md

# Specify vault
know fetch https://example.com/article --vault my-vault

# Clean up markdown formatting with LLM before saving
know fetch https://example.com/article --clean
```

## REST API

### Fetch and save a web page

```
POST /api/fetch
```

Request body:

```json
{
  "url": "https://example.com/article",
  "vault_id": "default",
  "path": "/articles/custom.md"
}
```

- `url` (required) — URL to fetch
- `vault_id` (optional) — target vault, defaults to first accessible vault
- `path` (optional) — custom save path, defaults to `/web/<slug>.md`
- `clean` (optional) — clean up markdown formatting with LLM before saving (default: `false`)

Response:

```json
{
  "path": "/web/example-article.md",
  "title": "Example Article",
  "vault_id": "default"
}
```

## MCP Tool

The `fetch_webpage` tool is available in the MCP server:

### Read-only mode (default)

Fetches the page and returns markdown without saving:

```
"Fetch https://example.com and summarize it"
```

### Save mode

Fetches and persists to the vault:

```
"Fetch https://example.com/guide and save it to my vault"
"Clip this article to /references/guide.md: https://example.com/guide"
```

### LLM cleanup

Use `clean=true` to pass the fetched markdown through an LLM that fixes formatting issues (broken headings, navigation remnants, malformed tables) without changing content:

```
"Fetch https://example.com/article, clean up the formatting, and save it"
```

Requires an LLM provider to be configured. Returns an error if no LLM is available and `clean` is requested.

### Example prompts

- "Fetch https://docs.example.com/api and summarize the authentication section"
- "Save this article to my vault: https://blog.example.com/post"
- "Fetch https://example.com/changelog and compare it with my notes in /notes/changelog.md"
- "Clip these pages and label them for later review: https://example.com/page1, https://example.com/page2"

## Vault Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `web_clip_path` | `/web` | Folder where clipped pages are saved |

Update via API:

```bash
curl -X PATCH /api/vaults/default/settings \
  -d '{"web_clip_path": "/clippings"}'
```
