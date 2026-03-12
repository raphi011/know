# Memory Decay System

How knowhow's project-specific memory system manages relevance over time through scoring, auto-archiving, and consolidation.

## Background: Why Decay?

Without decay, memory systems accumulate stale entries that dilute useful context. The AI agent receives outdated information alongside current knowledge, degrading response quality. Production memory systems (Mem0, Mnemosyne, Claude Code) all implement some form of decay to keep the memory pool relevant.

## Scoring Formula

Each memory gets a composite score combining two signals:

```
score = recency + access_boost
```

### Recency: Exponential Decay

```
recency = e^(-age_days / S)
```

Where:
- `age_days` = days since `last_accessed_at` (not `created_at` — accessing a memory refreshes it)
- `S` = strength constant (default: 30)

This is based on the **Ebbinghaus forgetting curve** (1885), which models human memory retention as exponential decay: `R = e^(-t/S)`. The formula has been validated across 100+ years of memory research and is used in spaced repetition systems (SuperMemo, Anki).

**Why S=30?** The Mnemosyne project (semantic memory for LLM systems) uses `e^(-age/30)` which gives a half-life of ~21 days (`ln(2) * 30 = 20.8`). This means:
- After 21 days without access: recency = 0.5
- After 42 days: recency = 0.25
- After 63 days: recency = 0.125
- After 90 days: recency = 0.05

This decay rate balances two needs: memories stay relevant long enough to be useful across sessions, but fade fast enough that truly abandoned knowledge doesn't persist indefinitely.

### Access Boost: Reinforcement

```
access_boost = min(access_count * 0.1, 2.0)
```

Each time a memory is retrieved, its `access_count` increments. Frequently accessed memories get a score boost, reflecting that they're actively useful.

**Why 0.1 per access?** Small enough that a single retrieval doesn't dominate the score, but meaningful enough that 5-10 accesses significantly extend a memory's active lifetime. The cap at 2.0 (20 accesses) prevents any single memory from becoming permanently immune to decay.

**Why cap at 2.0?** Without a cap, heavily-accessed memories would never archive regardless of age. The cap ensures even the most-accessed memory will eventually archive if not re-accessed (at score 2.0, it takes ~90 days to drop below 0.2).

## Score Examples

| Scenario | Age (days) | Access Count | Recency | Boost | Score |
|----------|-----------|-------------|---------|-------|-------|
| Just created | 0 | 1 | 1.00 | 0.10 | 1.10 |
| 1 week, accessed 2x | 7 | 2 | 0.79 | 0.20 | 0.99 |
| 1 month, accessed 3x | 30 | 3 | 0.37 | 0.30 | 0.67 |
| 2 months, accessed once | 60 | 1 | 0.14 | 0.10 | 0.24 |
| 2 months, accessed 5x | 60 | 5 | 0.14 | 0.50 | 0.64 |
| 3 months, accessed once | 90 | 1 | 0.05 | 0.10 | 0.15 |
| 3 months, accessed 10x | 90 | 10 | 0.05 | 1.00 | 1.05 |
| 6 months, accessed 20x | 180 | 20 | 0.00 | 2.00 | 2.00 |

## Archive Threshold

Memories with `score < 0.2` are auto-archived during retrieval.

**Why 0.2?** Based on the examples above:
- A memory accessed once survives ~65 days before archiving
- A memory accessed 5 times survives ~120 days
- A memory accessed 20 times (max boost) survives ~170 days

This means infrequently-used memories fade within 2 months, while actively useful ones persist for 4-6 months. These timelines match typical project work cycles.

The threshold is configurable per vault (`memory_archive_threshold`) so users can tune it. Lower values (e.g. 0.1) keep memories longer; higher values (e.g. 0.3) are more aggressive.

Archived memories are not deleted — they get an `archived` label and are excluded from default retrieval. They can still be retrieved with `include_archived=true` and found through general `search_documents`.

## Consolidation

When `retrieve_memories` is called, the system checks for redundant memories using embedding similarity.

### Merge Threshold: 0.95

Pairs of memories with cosine similarity > 0.95 are merge candidates. The server's LLM merges them into a single memory preserving all unique information.

**Why 0.95 (not lower)?** This is deliberately conservative:
- **0.95+** = near-duplicate (same info, slightly different wording). Safe to merge.
- **0.90** = high overlap but may have meaningful differences. Risk of losing nuance.
- **0.85** = related content, definitely should not be auto-merged.

It's much safer to under-merge than over-merge. Merging memories that shouldn't be merged is destructive and irreversible. The threshold is configurable per vault (`memory_merge_threshold`) for tuning.

### Merge Process

1. Compute pairwise cosine similarity among active memories using chunk embeddings
2. Pairs exceeding threshold become merge candidates
3. LLM generates a merged memory preserving unique information from both
4. Merged doc inherits: combined labels, summed access_count, current timestamp
5. Original memories are deleted

If no LLM is configured, consolidation is skipped entirely (graceful degradation).

## Design Decisions

### Why `last_accessed_at` instead of `created_at` for decay?

Using creation time would mean memories decay at a fixed rate regardless of usefulness. A memory accessed yesterday is relevant; one created 3 months ago and never re-accessed is probably stale. `last_accessed_at` lets active memories stay fresh.

### Why not vector similarity for retrieval scoring?

Project memories are typically few (tens, not thousands) and are already scoped by project label. A simple score-based sort is sufficient and avoids the complexity/cost of running embedding queries on every retrieval. The existing `search_documents` tool handles similarity-based retrieval for the broader document corpus.

### Why additive scoring (not multiplicative)?

Multiplicative (`recency * access_boost`) would mean a memory with zero access boost (access_count=0) always scores 0 regardless of recency. Additive ensures even a never-re-accessed memory starts with recency=1.0 and decays naturally.

## Sources

- [Mnemosyne: Semantic Memory for LLM Systems](https://rand.github.io/mnemosyne/) — `e^(-age/30)` decay, `+0.1` access boost, `max +2.0` cap
- [Mem0: Memory in Agents](https://mem0.ai/blog/memory-in-agents-what-why-and-how/) — recency + importance + similarity scoring
- [Mastering Memory Consistency in AI Agents (Sparkco)](https://sparkco.ai/blog/mastering-memory-consistency-in-ai-agents-2025-insights) — `relevance * 0.7 + recency * 0.3` weighting
- [Ebbinghaus Forgetting Curve (Wikipedia)](https://en.wikipedia.org/wiki/Forgetting_curve) — `R = e^(-t/S)` formula
- [SuperMemo: Exponential Nature of Forgetting](https://supermemo.guru/wiki/Exponential_nature_of_forgetting) — validation of exponential decay model
