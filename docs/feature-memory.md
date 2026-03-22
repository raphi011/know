# Memory

A project-scoped memory system that lets AI agents store and retrieve short notes about a project, with automatic decay scoring to keep the memory pool relevant over time.

## Overview

Memories are short documents stored under `/memories/` with an auto-generated date prefix and a `memory` label. They are designed for project-specific context that agents need across sessions -- deployment quirks, meeting decisions, API conventions, etc. A decay system based on the Ebbinghaus forgetting curve ensures stale memories fade out while actively used ones persist.

## How It Works

### Creating and Retrieving Memories

Memories are managed through three MCP tools:

| Tool | Description |
|------|-------------|
| `create_memory` | Create a project-scoped memory with auto date-prefix and `memory` label |
| `retrieve_memories` | Retrieve project memories sorted by relevance (with auto-archive and consolidation) |
| `delete_memory` | Delete a specific project memory |

When `retrieve_memories` is called, two maintenance operations run automatically:

1. **Auto-archive**: memories with a score below the archive threshold are moved out of the active set.
2. **Consolidation**: near-duplicate memories are merged into one.

### Decay Scoring

Each memory receives a composite score based on recency (Ebbinghaus forgetting curve) and access frequency. Memories that are accessed regularly stay fresh; unused ones decay over time. For the full algorithm, scoring formula, and design rationale, see [tech-memory.md](tech-memory.md).

### Archive Threshold

Memories with `score < 0.2` are auto-archived during retrieval. The threshold is configurable per vault via `memory_archive_threshold`. Lower values (e.g. 0.1) keep memories longer; higher values (e.g. 0.3) archive more aggressively.

Archived memories are not deleted. They receive an `archived` label and are excluded from default retrieval. They can still be retrieved with `include_archived=true` and found through `search_documents`.

### Consolidation

When `retrieve_memories` runs, near-duplicate memories (cosine similarity > 0.95) are automatically merged using the LLM. The merged document inherits combined labels and access counts. If no LLM is configured, consolidation is skipped. For implementation details, see [tech-memory.md](tech-memory.md).

## Usage

Example prompts for AI agents:

- "Remember that the deploy pipeline requires manual approval for production"
- "Save a note about today's meeting decisions, label it 'meetings' and 'project-x'"
- "Load my project memories for this repository"
- "Delete the memory about the old API endpoint format"

## Configuration

All memory settings are configurable per-vault via `know vault settings`:

```bash
know vault settings --set memory_path=/memories --set memory_decay_half_life=60
```

| Setting | Default | Description |
|---------|---------|-------------|
| `memory_path` | `/memories` | Folder for memory documents (per vault) |
| `memory_decay_half_life` | 30 | Half-life in days for the recency decay curve (per vault) |
| `memory_archive_threshold` | 0.2 | Score below which memories are auto-archived (per vault) |
| `memory_merge_threshold` | 0.95 | Cosine similarity above which memories are merge candidates (per vault) |
