# Phase 8: Maintenance Tools - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Maintenance operations users can run to keep the memory store healthy. Two capabilities: decay (reduce importance of unused entities) and similar pairs identification (find potential duplicates). Plus unit test coverage for query functions.

</domain>

<decisions>
## Implementation Decisions

### Decay behavior
- Affects BOTH importance_score AND search ranking (decayed entities rank lower)
- User: "You decide" on trigger mechanism, reversibility, output detail

### Similar pairs detection
- User: "You decide" on similarity metric, output content, threshold, merge behavior

### Tool interface design
- User: "You decide" on single vs multiple tools, dry-run mode, scope, batching

### Test coverage
- Integration tests with real SurrealDB (user decision)
- CI integration: separate integration step (unit tests in build, integration tests as separate job)
- User: "You decide" on test scope and test data management

### Claude's Discretion
- **Decay trigger:** Manual invocation with configurable threshold (e.g., entities not accessed in X days)
- **Decay reversibility:** Consider auto-restore on access for natural recovery
- **Decay output:** List affected entities with IDs and names (middle ground)
- **Similarity metric:** Embedding cosine similarity (semantic, already have embeddings)
- **Similarity threshold:** User-configurable with sensible default (e.g., 0.85)
- **Similarity output:** IDs + names + score (enough to decide without fetching)
- **Merge behavior:** Identify only (safer, user can merge manually via forget+remember)
- **Tool structure:** Single `reflect` tool with action parameter — simpler interface
- **Dry-run mode:** Yes, include dry_run flag for safety
- **Scope:** Context-scoped by default (matches other tools), global flag available
- **Batching:** Single operation per call (simpler, matches existing patterns)
- **Test scope:** Focus on query functions (db/queries.go) — highest value
- **Test data:** Setup/teardown per test for isolation

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 08-maintenance-tools*
*Context gathered: 2026-02-03*
