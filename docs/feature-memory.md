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

Each memory receives a composite score combining two signals:

```
score = recency + access_boost
```

**Recency** follows the Ebbinghaus forgetting curve:

```
recency = e^(-age_days / S)
```

- `age_days` = days since `last_accessed_at` (not `created_at` -- accessing a memory refreshes it)
- `S` = strength constant, default 30, giving a half-life of ~21 days

**Access boost** rewards frequently retrieved memories:

```
access_boost = min(access_count * 0.1, 2.0)
```

Each retrieval increments `access_count`. The cap at 2.0 (20 accesses) prevents any single memory from becoming permanently immune to decay.

### Score Examples

| Scenario | Age (days) | Access Count | Recency | Boost | Score |
|----------|-----------|-------------|---------|-------|-------|
| Just created | 0 | 1 | 1.00 | 0.10 | 1.10 |
| 1 week, accessed 2x | 7 | 2 | 0.79 | 0.20 | 0.99 |
| 1 month, accessed 3x | 30 | 3 | 0.37 | 0.30 | 0.67 |
| 2 months, accessed once | 60 | 1 | 0.14 | 0.10 | 0.24 |
| 3 months, accessed once | 90 | 1 | 0.05 | 0.10 | 0.15 |

### Archive Threshold

Memories with `score < 0.2` are auto-archived during retrieval. The threshold is configurable per vault via `memory_archive_threshold`. Lower values (e.g. 0.1) keep memories longer; higher values (e.g. 0.3) archive more aggressively.

Archived memories are not deleted. They receive an `archived` label and are excluded from default retrieval. They can still be retrieved with `include_archived=true` and found through `search_documents`.

### Consolidation

When `retrieve_memories` runs, the system checks for redundant memories using embedding similarity. Pairs with cosine similarity > 0.95 are merge candidates.

The merge process:

1. Compute pairwise cosine similarity among active memories using chunk embeddings.
2. Pairs exceeding the threshold become merge candidates.
3. The server's LLM generates a merged memory preserving unique information from both.
4. The merged document inherits combined labels, summed `access_count`, and a current timestamp.
5. Original memories are deleted.

If no LLM is configured, consolidation is skipped entirely.

## Usage

Example prompts for AI agents:

- "Remember that the deploy pipeline requires manual approval for production"
- "Save a note about today's meeting decisions, label it 'meetings' and 'project-x'"
- "Load my project memories for this repository"
- "Delete the memory about the old API endpoint format"

## Reference

### Design Decisions

**Why `last_accessed_at` instead of `created_at` for decay?**
Using creation time would mean memories decay at a fixed rate regardless of usefulness. A memory accessed yesterday is relevant; one created 3 months ago and never re-accessed is probably stale. `last_accessed_at` lets active memories stay fresh.

**Why not vector similarity for retrieval scoring?**
Project memories are typically few (tens, not thousands) and already scoped by project label. A simple score-based sort is sufficient and avoids the complexity and cost of running embedding queries on every retrieval. The existing `search_documents` tool handles similarity-based retrieval for the broader document corpus.

**Why additive scoring (not multiplicative)?**
Multiplicative (`recency * access_boost`) would mean a memory with zero access boost (`access_count=0`) always scores 0 regardless of recency. Additive ensures even a never-re-accessed memory starts with `recency=1.0` and decays naturally.

### Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `memory_archive_threshold` | 0.2 | Score below which memories are auto-archived (per vault) |
| `memory_merge_threshold` | 0.95 | Cosine similarity above which memories are merge candidates (per vault) |
