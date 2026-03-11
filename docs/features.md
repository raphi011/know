# Knowhow Features

Knowhow is a personal knowledge RAG database with hybrid search, automatic chunking, and LLM-powered synthesis. It exposes your knowledge through multiple interfaces — all backed by the same document service layer and SurrealDB storage.

## GraphQL API

The core API layer — all other interfaces (MCP, Web UI, CLI, iOS) are built on top of it. Served at `POST /query` with a playground at `/playground`.

### Auth

Bearer token in `Authorization` header, SHA256-hashed and looked up in DB. Falls back to share link token validation. A `noAuth` mode is available for local development.

### Queries

| Query | Description |
|-------|-------------|
| `me` | Current user with vault roles |
| `vault` / `vaults` | Vault lookup |
| `document` / `documentById` | Fetch document by path or ID |
| `search` | Hybrid BM25 + vector search with label/docType/folder filters |
| `templates` | List templates (optionally scoped to vault) |
| `conversations` / `conversation` | Chat conversation history |
| `vaultMembers` | List vault members (admin only) |
| `shareLinks` | List share links (admin only) |
| `syncMetadata` | Incremental metadata sync with `since` watermark |
| `documentVersions` / `versionDiff` | Version history and diffs |
| `serverConfig` | LLM/embedding/chunking configuration |

### Mutations

| Mutation | Description |
|----------|-------------|
| `createDocument` / `updateDocument` | Document CRUD |
| `moveDocument` / `deleteDocument` | Move or delete single documents |
| `moveDocumentsByPrefix` / `deleteDocumentsByPrefix` | Bulk path-prefix operations |
| `createFolder` / `deleteFolder` / `moveFolder` | Folder management |
| `rollbackDocument` | Restore a previous version |
| `createRelation` / `deleteRelation` | Typed document relationships |
| `createTemplate` / `deleteTemplate` | Template management |
| `createConversation` / `deleteConversation` / `renameConversation` | Conversation lifecycle |
| `addVaultMember` / `updateVaultMemberRole` / `removeVaultMember` | Vault member management (admin) |
| `createShareLink` / `deleteShareLink` | Share link management (admin) |
| `createUser` | Create user (system admin only) |

## MCP Server

An [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) server that gives AI agents direct access to your knowledge base. Served via Streamable HTTP (not stdio), authenticated with Bearer tokens.

### Tools

**Read-only:**

| Tool | Description |
|------|-------------|
| `search_documents` | Hybrid BM25 + vector search across all accessible vaults. Supports label, doc_type, folder filters |
| `get_document` | Retrieve document by path with full content, metadata, and content hash. Optional section outline |
| `list_labels` | List all labels/categories (cached 60s) |
| `list_folders` | List folder structure (cached 60s) |
| `list_folder_contents` | List immediate children of a folder |
| `get_document_versions` | Version history with timestamps, sources, and content hashes |

**Write:**

| Tool | Description |
|------|-------------|
| `create_memory` | Create short notes under `/memories/` with auto date-prefix and `memory` label |
| `create_document` | Create new document at a given path (fails if exists) |
| `edit_document` | Full content replacement with optimistic concurrency via `expected_hash` |
| `edit_document_section` | Section-level editing (replace, insert, delete, append) by heading — no need to send full content |

### Example Prompts

- "Search my knowledge base for everything about SurrealDB vector indexes"
- "Create a memory that the deploy pipeline requires approval from two reviewers"
- "Show me the version history of docs/architecture.md"
- "Edit the 'Prerequisites' section of my onboarding doc to include the new VPN step"

## Web UI

A server-rendered frontend built with Go [Templ](https://templ.guide/) templates and [HTMX](https://htmx.org/). No JavaScript framework, no Node.js build step.

### Pages

- **Agent chat** (`/agent`) — Conversational interface with streaming responses via SSE. Supports multi-turn conversations, conversation sidebar for history, and document references for context-aware answers.
- **Login** (`/login`) — Token-based authentication.
- **Settings** (`/settings`) — Vault switching, locale (EN/DE), theme (light/dark/system).

### Capabilities

- **Streaming chat**: Real-time token streaming with tool call visibility (search, read, create, edit)
- **Write tool approval**: When the agent wants to create or edit a document, a diff is shown for review. Approve all changes, select specific hunks, or reject
- **i18n**: English and German, stored as embedded JSON
- **Dark mode**: Light/dark/system theme with FOUC prevention
- **CSRF protection**: Origin/Referer header checking on all POST endpoints
- **Session management**: In-memory sessions with 24h TTL

## Internal Agent

An LLM-powered chat agent that can search, read, create, and edit documents in your knowledge base. Powers the Web UI chat and is also available via the GraphQL API.

### How It Works

1. User sends a message (optionally attaching document references for context)
2. The agent builds a system prompt including the vault's folder structure
3. The LLM decides which tools to call — searching, reading, or writing documents
4. Tool results feed back into the conversation until the LLM produces a final answer
5. Responses stream in real-time via SSE

### Tools

- `kb_search` — Hybrid search across the knowledge base
- `read_document` — Read a document's full content
- `list_labels` / `list_folders` / `list_folder_contents` — Browse the knowledge structure
- `create_document` / `edit_document` / `edit_document_section` — Write tools (require user approval)
- `web_search` — Web search via Tavily API (only after explicit user permission)

### Write Approval Flow

Write operations pause execution and emit an approval event with a computed diff. The user can:
- **Approve all** — apply all changes
- **Partial approve** — select specific diff hunks to apply
- **Reject** — discard the proposed changes

The agent then continues its reasoning loop with the outcome.

### Conversation Management

- Auto-creates conversations on first message
- Auto-titles conversations using the LLM after the first exchange
- Stores full message history (user, assistant, tool calls/results)

## WebDAV Server

A WebDAV server at `/dav/{vaultName}/` that lets you edit documents with any WebDAV-compatible editor. Documents are markdown files; folders map to the vault's folder structure.

### Auth

HTTP Basic Auth where the password is a knowhow API token. A `noAuth` mode is available for local development.

### Capabilities

- **Full CRUD**: Create, read, update, delete documents and folders
- **Document pipeline**: Writing a file triggers the full pipeline — parse, embed, link, chunk — on save
- **Markdown-only**: Only `.md` files can be created or renamed
- **Optimistic concurrency**: Content-hash ETags for conflict detection
- **macOS compatibility**: Silently handles `._*` resource forks and `.DS_Store`
- **Safety**: Rejects `PROPFIND Depth:infinity` (DoS prevention), enforces max PUT body size
- **Per-vault locking**: Each vault gets its own WebDAV lock system
- **Role-based access**: Write methods require `RoleWrite`; read-only users can browse and read

### Editor Support

Any WebDAV client works: macOS Finder, Transmit, Cyberduck, or VS Code with a WebDAV extension. Mount `/dav/default/` and edit markdown files directly — changes sync back through the full document pipeline.

## iOS App

A native SwiftUI app with offline support and real-time sync.

### Architecture

- **SwiftUI** with `NavigationSplitView` (sidebar + detail)
- **SwiftData** for local persistence (`CachedDocument` model)
- **XcodeGen** (`project.yml`) for project configuration

### Capabilities

- **Metadata-first sync**: Fetches document metadata first, content on demand. Incremental sync with `lastSyncedAt` watermark — only downloads changes
- **Content hash invalidation**: When a document's content hash changes, cached content is invalidated and re-fetched on next view
- **SSE live updates**: Real-time streaming of document changes (created, updated, deleted, moved) with auto-reconnect
- **Offline support**: `NWPathMonitor` detects connectivity; displays offline banner; cached documents remain accessible
- **Search**: Debounced (300ms) server-side search via GraphQL, results shown inline in the sidebar
- **Markdown rendering**: Rich document display using MarkdownUI
- **Multi-vault**: Loads all accessible vaults, each syncing independently
- **Keychain auth**: Token stored securely in Keychain with session restore on launch

### Screens

- **Login** — Token entry with server URL configuration
- **Sidebar** — Recursive folder tree with search, vault sections
- **Document view** — Rendered markdown with metadata (labels, dates, path)

## CLI

A command-line client (`cmd/knowhow/`) for bulk document ingestion. Communicates via the REST API.

### Copy Command

```bash
knowhow cp <local-dir> <vault-path> --vault <id> [flags]
```

Copies Markdown and image files from a local directory into a vault path. Uses the bulk upload endpoint for efficient single-request transfers. Unchanged files are skipped by content hash.

| Flag | Description |
|------|-------------|
| `--vault` | Target vault ID (required) |
| `-r, --recursive` | Recurse into subdirectories (default: false) |
| `--force` | Overwrite existing files if content hash differs |
| `--dry-run` | Preview without changes |
| `-l, --labels` | Comma-separated labels to apply |
| `--source` | Document source tag (default: `cp`) |
| `--api-url` | REST API base URL (default: `http://localhost:4001`, or `KNOWHOW_SERVER_URL`) |
| `--token` | API bearer token (or `KNOWHOW_TOKEN`) |

## Templates

Reusable document templates with optional AI flag. Templates can be scoped to a vault or shared globally.

| Field | Description |
|-------|-------------|
| `name` | Template name |
| `content` | Template body (markdown) |
| `description` | Optional description |
| `isAITemplate` | Distinguishes AI-generated templates from manual ones |
| `vaultId` | Optional vault scope (nil = global) |

### Example Prompts

- "Create a template for meeting notes with sections for attendees, agenda, and action items"
- "List all templates in my vault"

## Share Links

Public read-only links to documents or folders with optional expiration. Admin-only creation.

### How It Works

1. Admin creates a share link for a path, optionally with `expiresAt`
2. A random token is generated — the raw token is returned only on creation, the DB stores a SHA256 hash
3. Share tokens work as Bearer auth tokens, granting read-only access scoped to the specified vault + path
4. If `isFolder=true`, access extends to all documents under that path prefix

### Example Prompts

- "Create a share link for docs/onboarding that expires in 7 days"
- "List all share links for this vault"

## Vault Members

Multi-user collaboration with role-based access control. Three hierarchical roles:

| Role | Level | Capabilities |
|------|-------|-------------|
| `read` | 1 | Browse and read documents |
| `write` | 2 | Create, edit, delete documents |
| `admin` | 3 | Manage members, share links, vault settings |

Admins can add, update, and remove members. System admins bypass all role checks.

## Version History and Rollback

Every document update creates a new version with timestamp, source, and content hash. Versions are immutable — rollback creates a new version with the old content, preserving full history.

Rollback goes through the full document pipeline (parse, embed, link, chunk), so the restored content is fully indexed.

### Example Prompts

- "Show me all versions of docs/architecture.md"
- "Show the diff between the last two versions"
- "Roll back to the version from yesterday"

## Query Blocks

An inline DSL for embedding live queries inside documents. Results render as lists or tables.

### Syntax

````markdown
```knowhow
FROM /projects
WHERE labels CONTAIN "go"
WHERE type = "note"
WHERE title CONTAINS "setup"
SHOW title, labels, updated_at
SORT updated_at DESC
LIMIT 10
```
````

### Clauses

| Clause | Description |
|--------|-------------|
| `FROM <folder>` | Filter by folder path |
| `WHERE labels CONTAIN "<value>"` | Label membership |
| `WHERE type = "<value>"` | Exact doc_type match |
| `WHERE title CONTAINS "<value>"` | Substring match on title |
| `SHOW <fields>` | Columns to display (default: `title, path`) |
| `SORT <field> [ASC\|DESC]` | Sort order (default: `title ASC`) |
| `LIMIT <n>` | Max results (default: 50) |

**Format**: 1–2 SHOW fields renders as a **list** (bullet links), 3+ fields renders as a **table**.

## Document Relations

Typed relationships between documents, created via API or automatically from frontmatter.

### Sources

- **API**: `createRelation` mutation — requires write on source vault, read on target vault
- **Frontmatter**: Add `relates_to: path/to/doc` (or a list) to a document's frontmatter — relations are created automatically during the document pipeline

Relations are stored as SurrealDB graph edges with a unique constraint on `(from, to, rel_type)`.

## Sync API

Metadata-first incremental sync designed for offline-capable clients (used by the iOS app).

### How It Works

```graphql
syncMetadata(vaultId: ID!, since: DateTime, limit: Int, offset: Int): SyncMetadataResult
```

| Field | Description |
|-------|-------------|
| `documents` | List of `{ id, path, contentHash, updatedAt }` — no content |
| `tombstones` | Deleted documents `{ docId, path, deletedAt }` (only with `since`) |
| `hasMore` | Pagination flag (default limit: 500) |

- **Without `since`**: Full metadata dump (no tombstones)
- **With `since`**: Only documents updated after that timestamp + tombstones for deletions

Clients compare `contentHash` values against their local cache and fetch full content only for changed documents.
