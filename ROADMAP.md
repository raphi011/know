# Memory MCP Server: Feature Roadmap

## Planned Features

### Phase 1 (No LLM Required)
1. **Episodic Memory** - Store complete sessions as first-class objects
2. **Project Namespacing** - Isolate memories by project/context
3. **Importance Scoring** - Heuristic-based salience scoring

### Phase 2 (Requires MCP Sampling or API)
*Postponed until Claude Code supports MCP sampling or we integrate direct API calls*
- Memory consolidation/summarization
- LLM-generated importance scores
- Auto-entity extraction from episodes

---

## Critical Files

| File | Changes |
|------|---------|
| `memcp/db.py` | Schema, queries, importance calc |
| `memcp/models.py` | EpisodeResult, EntityResult updates |
| `memcp/servers/episode.py` | **NEW** - episode tools |
| `memcp/servers/search.py` | context + importance params |
| `memcp/servers/persist.py` | context + importance params |
| `memcp/servers/maintenance.py` | context + recalc_importance |
| `memcp/server.py` | mount episode_server |

---

## Feature 1: Episodic Memory

### New Schema (db.py)
```sql
DEFINE TABLE episode SCHEMAFULL;
DEFINE FIELD content ON episode TYPE string;
DEFINE FIELD embedding ON episode TYPE array<float>;
DEFINE FIELD timestamp ON episode TYPE datetime;
DEFINE FIELD summary ON episode TYPE option<string>;
DEFINE FIELD metadata ON episode TYPE object;
DEFINE FIELD context ON episode TYPE option<string>;
-- Indexes: timestamp, context, HNSW embedding, BM25 fulltext

DEFINE TABLE extracted_from TYPE RELATION IN entity OUT episode SCHEMAFULL;
```

### New Tools
- `add_episode(content, timestamp?, summary?, metadata?, context?, entity_ids?)` - store session
- `search_episodes(query, time_start?, time_end?, context?, limit)` - temporal + semantic search
- `get_episode(episode_id, include_entities?)` - retrieve with linked entities

---

## Feature 2: Project Namespacing

### Schema Change
```sql
DEFINE FIELD context ON entity TYPE option<string>;
DEFINE INDEX entity_context ON entity FIELDS context;
```

### Context Detection (db.py)
```python
def detect_context(explicit: str | None) -> str | None:
    if explicit: return explicit
    if MEMCP_DEFAULT_CONTEXT: return MEMCP_DEFAULT_CONTEXT
    if MEMCP_CONTEXT_FROM_CWD: return os.path.basename(os.getcwd())
    return None
```

### Tool Changes
Add `context` param to: `search`, `remember`, `traverse`, `find_path`, `reflect`

### New Tools
- `list_contexts()` - list all namespaces
- `get_context_stats(context)` - entity/episode counts

---

## Feature 3: Importance Scoring (Heuristic-Based)

### Schema Change
```sql
DEFINE FIELD importance ON entity TYPE float DEFAULT 0.5;
DEFINE FIELD user_importance ON entity TYPE option<float>;
```

### Importance Formula (No LLM)
```
importance = 0.3 * connectivity_score   -- min(1, relations/10)
           + 0.3 * access_score         -- min(1, log10(access_count+1)/3)
           + 0.4 * user_importance      -- user-set or 0.5 default
```

### Weighted Search (db.py)
```
final_score = rrf_score * 0.7 + importance * 0.2 + recency * 0.1
```

### Tool Changes
- `remember()` - accept `importance` field in entities
- `search()` - add `use_importance`, `importance_weight`, `recency_weight`
- `reflect()` - add `recalculate_importance` flag

---

## Implementation Order

### Phase 1: Schema (db.py)
1. Episode table + extracted_from relation
2. Add `context` to entity
3. Add `importance`/`user_importance` to entity
4. All indexes

### Phase 2: Query Functions (db.py)
1. Episode CRUD queries
2. Update existing queries for `context` param
3. Importance calculation functions
4. Weighted search function

### Phase 3: Models (models.py)
1. Add EpisodeResult, EpisodeSearchResult
2. Update EntityResult (context, importance)
3. Update ReflectResult

### Phase 4: New Episode Server
1. Create `servers/episode.py`
2. add_episode, search_episodes, get_episode

### Phase 5: Update Existing Servers
1. search.py - context + importance
2. persist.py - context + importance
3. maintenance.py - context + recalc
4. graph.py - context

### Phase 6: Main Server
1. Mount episode_server
2. Add memory://contexts resource

### Phase 7: Tests
1. Episode tests
2. Context filtering tests
3. Importance tests

---

## Future Features (Research)

| Feature | Difficulty | Value | Requires LLM |
|---------|-----------|-------|--------------|
| Entity type ontology | Low | Medium | No |
| Observations model | Medium | Medium | No |
| Smart forgetting | Medium | Medium | Optional |
| Bi-temporal model | High | Medium | No |
| Procedural memory | Low | Medium | No |
| Memory consolidation | Medium | High | **Yes** |
| Fact versioning | High | Low | No |

---

## Sources

- [Graphiti by Zep](https://github.com/getzep/graphiti)
- [Zep Knowledge Graph MCP](https://www.getzep.com/product/knowledge-graph-mcp/)
- [MCP Knowledge Graph](https://github.com/shaneholloman/mcp-knowledge-graph)
- [IBM AI Agent Memory](https://www.ibm.com/think/topics/ai-agent-memory)
- [Mem0 Memory in Agents](https://mem0.ai/blog/memory-in-agents-what-why-and-how)
