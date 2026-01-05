# Development Status

## Current Session Summary (2026-01-05)

### What We Were Doing

**Primary Goal:** Adding integration tests for SurrealDB query functions extracted to `db.py`

**Progress:**
- Created 7 passing integration tests (out of 22 query functions)
- Discovered KNN operator issue during testing
- Researched and resolved MTREE deprecation

### Tests Completed (7/22)

✅ Passing tests:
1. `test_connection` - Database connection and schema initialization
2. `test_query_upsert_and_get_entity` - Entity CRUD operations
3. `test_query_delete_entity` - Entity deletion
4. `test_query_list_labels` - Label aggregation and deduplication
5. `test_query_update_access` - Access count increment
6. `test_query_create_relation` - Relation creation between entities
7. `test_query_similar_entities` - Vector similarity search

### Major Issue Discovered and Resolved

**Problem:** KNN operator `<|k,ef|>` returned empty results even with valid MTREE index

**Root Cause:** MTREE index was deprecated and completely removed from SurrealDB v3.0 (November 2025)
- PR #6553: MTREE removed
- Issue #6598: Confirmed MTREE no longer works
- Replacement: HNSW (Hierarchical Navigable Small World) index

**Resolution:**
1. ✅ Updated schema: `MTREE` → `HNSW` in `memcp/db.py:43`
2. ✅ Fixed 3 queries with KNN operator syntax:
   - `query_hybrid_search` (line 219): `<|k,100|>` → `<|k,COSINE|>`
   - `query_similar_by_embedding` (line 395): `<|k,100|>` → `<|k,COSINE|>`
   - `query_similar_for_contradiction` (line 421): `<|10,100|>` → `<|10,COSINE|>`
3. ✅ Updated CLAUDE.md with MTREE deprecation timeline and migration guide
4. ✅ Committed changes (commit: 8b5be98)

### Where We Left Off

**Next Steps:**

1. **Upgrade SurrealDB to v3.0.0-beta.1** (User is handling this)
   ```bash
   docker run -p 8000:8000 surrealdb/surrealdb:v3.0.0-beta.1 \
     start --user root --pass root memory
   ```

2. **Test with v3.0.0-beta.1** (After upgrade completes)
   - Run existing test suite: `uv run pytest memcp/test_db.py -v`
   - Verify KNN operator now works with HNSW
   - Update investigation test (`test_knn_operator_investigation`) to document success
   - Verify all 7 tests still pass

3. **Cleanup**
   - Remove or update the KNN investigation test
   - Consider adding test for one of the fixed KNN queries

4. **Continue Adding Tests** (15 remaining query functions)
   - `query_hybrid_search` - Hybrid BM25 + vector search
   - `query_traverse` - Graph traversal
   - `query_find_path` - Path finding
   - `query_apply_decay` - Temporal decay
   - `query_all_entities_with_embedding` - Bulk retrieval
   - `query_similar_by_embedding` - Now using HNSW KNN
   - `query_delete_entity_by_record_id` - Deletion by record ID
   - `query_entity_with_embedding` - Single entity retrieval
   - `query_similar_for_contradiction` - Now using HNSW KNN
   - `query_entities_by_labels` - Label filtering
   - `query_vector_similarity` - Similarity calculation
   - `query_count_entities` - Entity count
   - `query_count_relations` - Relation count
   - `query_get_all_labels` - All labels retrieval
   - `query_count_by_label` - Count by label

5. **Final Goal:** Update `server.py` to use extracted query functions (after all tests pass)

## Key Files Modified

- `memcp/db.py` - Schema + 3 KNN queries updated for HNSW
- `memcp/test_db.py` - 7 integration tests + KNN investigation test
- `CLAUDE.md` - Added MTREE deprecation documentation

## Technical Decisions Made

1. **Use HNSW instead of MTREE** for SurrealDB v3.0+ compatibility
2. **KNN syntax change:** `<|k,ef|>` → `<|k,DISTANCE|>` (COSINE, EUCLIDEAN, etc.)
3. **Fallback approach:** `vector::similarity::cosine()` works without index on all versions
4. **Test strategy:** Real SurrealDB integration tests, not mocks

## Commands Reference

```bash
# Run all DB tests
uv run pytest memcp/test_db.py -v

# Run specific test
uv run pytest memcp/test_db.py::test_query_similar_entities -v

# Run with output
uv run pytest memcp/test_db.py -v -s

# Build verification test
uv run pytest test_memcp.py -v
```

## SurrealDB Version Requirement

- **Minimum:** v3.0.0-beta.1 (for HNSW support)
- **Recommended:** Latest v3.0.0 pre-release or stable when available
- **Not supported:** v2.x (uses deprecated MTREE)
