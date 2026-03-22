# Memory System — Architecture

Technical reference for the memory system's decay scoring algorithm, consolidation logic, and design decisions. For user-facing usage and configuration, see [feature-memory.md](feature-memory.md).

## Decay Scoring

Each memory receives a composite score combining two signals:

```
score = recency + access_boost
```

**Recency** follows the Ebbinghaus forgetting curve:

```
recency = e^(-age_days / S)
```

- `age_days` = days since `last_accessed_at` (not `created_at` — accessing a memory refreshes it)
- `S` = half-life in days (default 30), configurable per vault via `memory_decay_half_life`

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

## Consolidation

When `retrieve_memories` runs, the system checks for redundant memories using embedding similarity. Pairs with cosine similarity > 0.95 are merge candidates.

The merge process:

1. Compute pairwise cosine similarity among active memories using chunk embeddings.
2. Pairs exceeding the threshold become merge candidates.
3. The server's LLM generates a merged memory preserving unique information from both.
4. The merged document inherits combined labels, summed `access_count`, and a current timestamp.
5. Original memories are deleted.

If no LLM is configured, consolidation is skipped entirely.

## Design Decisions

**Why `last_accessed_at` instead of `created_at` for decay?**
Using creation time would mean memories decay at a fixed rate regardless of usefulness. A memory accessed yesterday is relevant; one created 3 months ago and never re-accessed is probably stale. `last_accessed_at` lets active memories stay fresh.

**Why not vector similarity for retrieval scoring?**
Project memories are typically few (tens, not thousands) and already scoped by project label. A simple score-based sort is sufficient and avoids the complexity and cost of running embedding queries on every retrieval. The existing `search_documents` tool handles similarity-based retrieval for the broader document corpus.

**Why additive scoring (not multiplicative)?**
Multiplicative (`recency * access_boost`) would mean a memory with zero access boost (`access_count=0`) always scores 0 regardless of recency. Additive ensures even a never-re-accessed memory starts with `recency=1.0` and decays naturally.
