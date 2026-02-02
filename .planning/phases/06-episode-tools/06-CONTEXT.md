# Phase 6: Episode Tools - Context

**Gathered:** 2026-02-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Store and search episodic memories (time-stamped experiences/events). Tools: add_episode, search_episodes, get_episode, delete_episode. Episodes are distinct from entities — they represent temporal experiences rather than facts/knowledge.

</domain>

<decisions>
## Implementation Decisions

### Episode structure
- Auto-generate embeddings on episode creation (required for semantic search)
- Research Python implementation for field structure and metadata

### Context linking
- Episodes use same context detection as entities: explicit > config > git origin > cwd
- Every episode has a context field (required, same pattern as entities)
- Support cross-reference option: flag to include related episodes when fetching entities
- Entity linking approach left to Claude's discretion

### Search behavior
- Use hybrid search (BM25 + vector RRF fusion) — same approach as entity search
- Support time-range filtering with before/after date params
- Boost recent episodes in ranking (recency affects relevance)
- Return full episode content in search results

### Temporal handling
- Timestamps are auto-generated at creation time (no user-provided timestamps)
- Second precision (standard datetime)
- Time-range query format left to Claude's discretion (ISO vs relative)

### Claude's Discretion
- Episode content structure (single text vs structured blocks)
- Context field: optional vs required (research Python model)
- Episode metadata fields beyond content/timestamp
- Whether to track updated_at (episodes may be immutable)
- Entity auto-linking on episode creation
- Time-range query format (ISO strings vs relative durations)

</decisions>

<specifics>
## Specific Ideas

- Episodes should feel like "memories of experiences" vs entities which are "facts"
- Cross-reference feature: when getting an entity, optionally see episodes that mentioned it

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-episode-tools*
*Context gathered: 2026-02-02*
