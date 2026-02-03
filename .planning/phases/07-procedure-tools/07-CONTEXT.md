# Phase 7: Procedure Tools - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

CRUD and list operations for procedural memory (how-to knowledge). Users can create procedures with ordered steps, search/retrieve/delete procedures by ID, and list all procedures. Procedure execution, versioning, and step-level operations are out of scope.

</domain>

<decisions>
## Implementation Decisions

### Search behavior
- Use hybrid search (BM25 + vector) with RRF fusion — same pattern as entities and episodes
- Consistency with existing search implementations is the priority

### Claude's Discretion
The following areas were discussed but left to Claude's judgment based on Python implementation patterns, consistency with existing code, and best practices:

**Step structure:**
- Plain text array vs structured objects
- Optional metadata per step (notes/warnings)
- Validation rules (empty steps allowed or not)
- Minimum steps required (1 or 2)

**Procedure metadata:**
- Fields beyond title/description/steps (tags, prerequisites)
- Context field handling (required, optional, or none)
- Usage tracking (access count, last accessed)
- ID generation strategy (auto vs user-provided)

**Search behavior:**
- What content to search (title, description, steps, or combination)
- Context filtering approach
- Default result limit

**List & filtering:**
- List output detail level (ID+title vs full procedures)
- Pagination support
- Context auto-detection
- Default sort order

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. Follow existing patterns from entity and episode implementations.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 07-procedure-tools*
*Context gathered: 2026-02-03*
