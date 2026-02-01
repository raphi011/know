# Codebase Concerns

**Analysis Date:** 2026-02-01

## Tech Debt

### SurrealDB Python SDK CBOR Type Support

**Issue:** Python SDK doesn't support CBOR tag 56 for `set<T>` type. Working around by using `array<T>` instead.

**Files:** `memcp/db.py` (lines 167, 243, 588, 1120)

**Impact:** Schema uses suboptimal array types where sets would be semantically correct. Queries must call `array::distinct()` to deduplicate labels. When Python SDK adds support, this code must be refactored.

**Fix approach:** Once [surrealdb.py issue](https://github.com/surrealdb/surrealdb.py/issues) resolves CBOR tag 56, update schema to use `set<string>` for labels fields:
- `entity.labels` → `set<string>`
- `procedure.labels` → `set<string>`
Remove redundant `array::distinct()` calls in queries.

### Missing MCP Sampling Support

**Issue:** Claude Code doesn't support MCP sampling (Issue #1785). Features were removed that relied on LLM integration.

**Files:** `Claude.md` (lines 42-62)

**Removed features:**
- `memorize_file` tool (was using LLM to extract entities from documents)
- `auto_tag` parameter in `remember` (was using LLM to generate labels)
- `summarize` parameter in `search` (was using LLM to summarize results)

**Impact:** Users cannot automatically extract entities from documents or auto-generate labels. Tool capabilities are reduced.

**Fix approach:** Two options when/if Claude Code adds sampling:
1. Re-implement using original `sample()` function in `memcp/utils/embedding.py`
2. Call Anthropic API directly with ANTHROPIC_API_KEY env var instead of relying on MCP sampling

### Missing Docling Integration

**Issue:** No document parsing library for PDF, DOCX, PPTX, images, scanned docs.

**Files:** None yet (planned feature)

**Impact:** `memorize_file` tool cannot handle various document formats. Users must pre-process documents to text.

**Fix approach:** Add [docling](https://github.com/docling-project/docling) for document parsing:
```bash
uv add docling
```
Workflow: Documents → markdown/text → extract entities → store in SurrealDB.

## Known Bugs

### SDK Race Condition on Connection Close

**Issue:** Calling `db.close()` hangs due to race condition in Python SDK.

**Files:** `test_integration.py` (lines 46-48)

**Trigger:** When closing connection, SDK's `_recv_task` race condition causes hang.

**Workaround:** Don't call `db.close()`. Connection is cleaned up when process exits. This is intentional - documented in code with comment.

**Impact:** Cannot gracefully close connections in tests. No way to detect/report close failures.

### SDK CBOR Decode Errors on Query Response

**Issue:** SDK crashes with `CancelledError` or `KeyError` when SurrealDB response contains unsupported CBOR types (e.g., set type).

**Files:** `memcp/db.py` (lines 385-403)

**Trigger:** When query response includes `set<T>` type and schema tries to return it.

**Symptoms:**
- `asyncio.CancelledError` - SDK connection broken during CBOR decode
- `KeyError` on query ID - `_recv_task` crashed during decode

**Current mitigation:** Schema avoids `set<T>` types. Error handling converts SDK errors to user-friendly `ToolError` messages.

**Impact:** Queries fail unpredictably if schema incorrectly uses sets. Hard to debug.

## Security Considerations

### Default SurrealDB Credentials

**Issue:** Default credentials (`root`/`root`) are used when env vars not set.

**Files:** `memcp/db.py` (lines 26-31)

**Risk:** Running without proper authentication in production allows unauthorized access.

**Current mitigation:** Env var configuration with defaults only for development.

**Recommendations:**
- Require explicit auth env vars in production (fail fast if missing)
- Log warnings if default credentials are used
- Document security setup in README for deployment
- Use secrets management (1Password, Vault, etc.) in production

### Bare Exception Handling

**Issue:** Broad `except Exception:` blocks suppress error details and may hide security issues.

**Files:**
- `memcp/db.py` (line 131) - Silent exception in git origin parsing
- `memcp/db.py` (line 355) - Silent exception on DB close
- `memcp/api/main.py` (line 90) - Silent exception in count extraction
- `memcp/api/schema.py` (line 202) - Silent exception on datetime parse

**Impact:** Production issues hidden. Malformed data silently ignored. Difficult to troubleshoot.

**Recommendations:**
- Add specific exception types (not bare `Exception`)
- Log caught exceptions with context
- Consider whether silent failures are appropriate or should propagate

### Hardcoded Embedding Dimension

**Issue:** HNSW embedding indices hardcoded to 384 dimensions matching `all-MiniLM-L6-v2` model.

**Files:** `memcp/db.py` (lines 185, 218, 251)

**Risk:** If embedding model changes, dimension mismatch breaks vector search silently.

**Recommendations:**
- Extract to env var or constant: `EMBEDDING_DIMENSION = 384`
- Validate embedding dimensions at runtime before storing
- Add schema validation that new embeddings match stored dimension

## Performance Bottlenecks

### Full-Text Search Tokenization

**Issue:** BM25 FT indices use English-only tokenizer and filters.

**Files:** `memcp/db.py` (lines 186, 187, 219, 220, 252, 253)

```sql
DEFINE ANALYZER entity_analyzer TOKENIZERS class FILTERS lowercase, ascii, snowball(english)
```

**Problem:** Non-English text, accented characters, and Unicode are normalized to ASCII, losing nuance.

**Current capacity:** Small datasets (< 1M entities) perform fine.

**Improvement path:**
- Add language detection to route to appropriate analyzer
- Support multi-language tokenizers (e.g., `snowball(french)`, `snowball(spanish)`)
- Consider specialized NLP libs for specific languages

### Embedding LRU Cache Size

**Issue:** 1,000 entry cache (~1.5MB) may be too small for large knowledge bases.

**Files:** `memcp/utils/embedding.py` (line 55)

```python
@lru_cache(maxsize=1000)
def _embed_cached(text: str) -> tuple[float, ...]:
```

**Current capacity:** 1,000 unique texts before eviction.

**Scaling limit:** High eviction rate with > 10k unique entity contents.

**Improvement path:**
- Make maxsize configurable: `EMBEDDING_CACHE_SIZE = int(os.getenv("EMBEDDING_CACHE_SIZE", "1000"))`
- Monitor cache hit rate via `get_embed_cache_stats()`
- Consider disk-backed cache for persistent models

### Lazy Model Loading Thread Contention

**Issue:** Models load lazily on first use. High concurrency creates thread contention.

**Files:** `memcp/utils/embedding.py` (lines 19-52)

**Current mitigation:** `preload_models()` function can be called at startup to warm up.

**Impact:** First queries in production may experience 2-3s delay while models load.

**Improvement path:**
- Call `preload_models()` in server startup handler
- Add timeout to model loading with fallback
- Monitor model load time in logs

## Fragile Areas

### Database Query Parsing

**Files:** `memcp/db.py` (lines 365-403)

**Why fragile:** Raw string concatenation for SQL queries with variable substitution. Query results are cast without validation.

```python
return cast(QueryResult, result)
```

**Risk:** Type mismatches between query result and expected type go unnoticed.

**Safe modification:** Use type guards before returning:
```python
if not isinstance(result, list):
    raise ToolError(f"Expected list, got {type(result)}")
```

### Context Detection from Git

**Files:** `memcp/db.py` (lines 96-157)

**Why fragile:** Subprocess call to `git remote get-url origin` can fail for:
- Non-git directories
- Repos without remotes
- Git command not found
- Permission errors

Exception is silently caught (line 131).

**Safe modification:**
- Add logging on git command failure
- Add explicit fallback to cwd basename
- Test with non-git directories

### Graph Traversal Depth Limits

**Files:** `memcp/servers/graph.py` (lines 25, 47)

**Why fragile:** Depth validation is hardcoded:
```python
raise ToolError("depth must be between 1 and 10")
raise ToolError("max_depth must be between 1 and 20")
```

**Risk:** If graph becomes large, depth limits may be too restrictive or too permissive. No analysis of actual graph structure.

**Recommendation:** Make limits configurable with env vars and document reasoning.

### Global Database Connection in API

**Files:** `memcp/api/main.py` (lines 70-78, 521)

**Why fragile:** Single global `_db` variable shared across all requests.

```python
_db: "AsyncSurreal | None" = None
```

**Risk:** No connection pooling. If connection breaks, all requests fail until restart.

**Safe modification:**
- Replace with connection pool (e.g., `asynccontextmanager` that creates new connections)
- Add connection health checks
- Implement reconnection logic on connection loss

## Scaling Limits

### Single Database Connection

**Issue:** API uses single global connection instead of connection pool.

**Current capacity:** Works for < 100 concurrent requests. Beyond that, connection becomes bottleneck.

**Limit:** Connection breaks → entire service fails.

**Scaling path:**
1. Replace global `_db` with async connection pool
2. Add connection recycling (close/reopen periodically)
3. Add health checks and automatic reconnection

### Embedding Model Memory

**Issue:** Both embedding and NLI models loaded into memory simultaneously.

**Files:** `memcp/utils/embedding.py`

**Current capacity:** ~2GB total memory. Works for single instance.

**Limit:** OOM on memory-constrained environments.

**Scaling path:**
1. Externalize embedding service (e.g., via API endpoint)
2. Add optional NLI model disabling
3. Use smaller models (`all-MiniLM-L6-v2` is already small)

### Full-Text Search Index Growth

**Issue:** BM25 FT indices grow with every entity added.

**Current capacity:** Unknown limit. SurrealDB docs don't specify.

**Scaling consideration:** Monitor index size growth. May need periodic vacuum/optimization.

## Dependencies at Risk

### sentence-transformers Version Pinning

**Issue:** `sentence-transformers` has breaking changes between versions. Not pinned in `pyproject.toml`.

**Files:** `pyproject.toml`

**Risk:** `pip install` on new machine gets different version, model loading fails.

**Mitigation:** Version constraints should be documented. Review actual constraints in `uv.lock`.

**Recommendation:**
- Pin major version: `sentence-transformers>=2.2,<3`
- Test with minimum version regularly

### SurrealDB SDK Pre-1.0 Status

**Issue:** Python SDK is < v1.0 (unstable). API may change.

**Files:** All database code in `memcp/db.py`

**Risk:** SDK updates could break queries or introduce new bugs.

**Example:** Known CBOR tag 56 issue, connection close hang, CBOR decode crashes.

**Recommendation:**
- Monitor SDK releases closely
- Test SDK updates in staging before deploy
- Document SDK version constraints in `pyproject.toml`

## Missing Critical Features

### No Connection Retry Logic

**Issue:** Database connection fails → server crashes immediately.

**Files:** `memcp/db.py` (lines 280-327), `memcp/api/main.py` (lines 524-545)

**Problem:** No exponential backoff or retry attempts.

**Blocks:** Production deployment in unreliable networks.

**Recommendation:**
- Add configurable retry with exponential backoff
- Add max retry count before giving up
- Log retry attempts for debugging

### No Query Result Validation

**Issue:** Query results are cast to expected type without validation.

**Files:** `memcp/db.py` (line 382) - `cast(QueryResult, result)`

**Problem:** If SurrealDB schema changes, type mismatches go unnoticed until runtime.

**Blocks:** Safe schema evolution.

**Recommendation:**
- Add runtime type checking with `isinstance()` or Pydantic models
- Validate result structure before returning to callers

### No Query Timeout Feedback

**Issue:** Query timeout set to 30s globally but no per-tool timeout strategy.

**Files:** `memcp/db.py` (lines 281, 325)

**Problem:** Some tools might benefit from shorter timeouts (e.g., search 5s). Others might need longer (e.g., large graph traversal 60s).

**Blocks:** Tuning timeouts per-tool.

**Recommendation:**
- Add optional timeout parameter to `run_query()`
- Make timeouts tool-specific via env vars

## Test Coverage Gaps

### No Mocking of Embedding Models

**Issue:** Tests that use embedding must load real models (slow, 2-3s per test).

**Files:** `memcp/test_e2e.py`, `test_integration.py`

**Risk:** Tests are slow. CI/CD feedback loop is slow.

**Recommendation:**
- Mock `embed()` and `check_contradiction()` in unit tests
- Only load real models in integration/E2E tests
- Add `@pytest.mark.slow` to skip during fast test runs

### No Error Path Testing

**Issue:** Many error conditions not tested:
- SDK CBOR decode failures
- Connection timeouts
- Malformed entity data
- Missing database fields

**Files:** `test_integration.py`, `test_e2e.py`

**Risk:** Error handling code untested. May fail in production.

**Recommendation:**
- Add error injection tests (mock SDK to raise errors)
- Test all exception handlers
- Test edge cases (empty results, null fields, missing entities)

### No Load Testing

**Issue:** No tests for:
- Concurrent queries
- Large result sets (1000+ entities)
- Slow SurrealDB responses
- Query timeouts under load

**Risk:** Performance characteristics unknown until production.

**Recommendation:**
- Add pytest-benchmark for performance regression testing
- Add locust/k6 for load testing
- Test with 10k+ entities in staging

---

*Concerns audit: 2026-02-01*
