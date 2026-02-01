# Phase 4: Persistence Tools - Context

**Gathered:** 2026-02-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Store and delete entities and relations via MCP tools. Users can persist new entities with auto-generated embeddings, create relations between entities, and delete entities by ID. Graph traversal and search are handled by other phases.

</domain>

<decisions>
## Implementation Decisions

### Entity creation behavior
- Upsert semantics: if entity with same name exists, update it
- Required fields: name and content (type, labels, observations optional)
- Labels on upsert: merge (additive) — new labels added, never removed
- Entity name uniqueness: scoped per context (same name allowed in different contexts)

### Relation handling
- Duplicate relation policy: update metadata on existing relation (don't create duplicates)
- Relation types: freeform strings (no predefined constraint set)

### Response format
- Embeddings: never include in responses (internal implementation detail)

### Claude's Discretion
- Self-relations: whether to allow entity relating to itself
- Missing entity on relation: error vs auto-create stub
- Response verbosity: ID-only vs full entity on remember success
- Upsert action indicator: whether response indicates created vs updated
- Forget behavior:
  - Cascade vs error on existing relations
  - Soft vs hard delete
  - Batch delete support (array of IDs)
  - Non-existent entity handling (silent success vs error)
  - Response format (message vs deleted entity summary)

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches matching Python implementation patterns.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 04-persistence-tools*
*Context gathered: 2026-02-01*
