# memcp Web Dashboard Implementation Plan

## Summary
Build a Python-based SPA dashboard using **NiceGUI** that matches the React mock exactly. Reuses existing `db.py` functions via a lightweight context adapter.

## Reference Mock
**Location:** `~/Downloads/memcp-control-plane/`

Full React/TypeScript implementation with all screens:
- `screens/Overview.tsx` - Dashboard
- `screens/Search.tsx` - Memory search
- `screens/GraphView.tsx` - Entity graph (Cytoscape.js)
- `screens/EntityDetail.tsx` - Entity detail view
- `screens/AddMemory.tsx` - Add memory form
- `screens/Ingest.tsx` - File upload
- `screens/Maintenance.tsx` - Reflect/cleanup
- `screens/Episodes.tsx` + `EpisodeDetail.tsx` - Episodic memory
- `screens/Procedures.tsx` + `ProcedureEditor.tsx` - Procedural memory

Data models: `types.ts`
Mock backend: `backend.ts`

## Framework: NiceGUI
- Pure Python, Vue-based reactivity
- Built-in Tailwind CSS support (matches dark theme design)
- Direct db.py function reuse (no REST layer needed)
- Single Docker container deployment
- Plotly for charts, Cytoscape.js embedded for graph

## Project Structure
```
memcp/webui/
├── __init__.py
├── main.py              # NiceGUI app entry point
├── theme.py             # Dark theme CSS/Tailwind config
├── pages/
│   ├── dashboard.py     # Overview - stats, charts, activity
│   ├── search.py        # Memory search
│   ├── graph.py         # Entity graph (Cytoscape.js)
│   ├── entity.py        # Entity detail view
│   ├── add.py           # Add memory form
│   ├── upload.py        # File upload/ingest
│   ├── maintenance.py   # Reflect/cleanup
│   ├── episodes.py      # Episodes list + detail
│   └── procedures.py    # Procedures list + editor
└── components/
    ├── sidebar.py       # Sidebar with context selector
    ├── stat_card.py     # Metric cards
    ├── entity_card.py   # Search result cards
    └── graph_view.py    # Cytoscape.js wrapper
```

## Key Implementation Details

### 1. Refactor `db.py` to be Context-Independent

Current signature: `async def query_*(ctx: Context, ...) -> QueryResult`
New signature: `async def query_*(db: AsyncSurreal, ...) -> QueryResult`

**Changes to `db.py`:**
```python
# Before
async def run_query(ctx: Context, sql: str, vars: dict | None = None) -> QueryResult:
    db = get_db(ctx)
    ...
    await ctx.error(f"Query failed: {e}")

# After
async def run_query(db: AsyncSurreal, sql: str, vars: dict | None = None) -> QueryResult:
    ...
    logger.error(f"Query failed: {e}")  # Use logger instead of ctx.error
```

**MCP server wrapper** (keeps existing tool signatures working):
```python
# memcp/server.py - thin wrapper for MCP tools
async def query_hybrid_search_mcp(ctx: Context, query: str, ...) -> QueryResult:
    db = get_db(ctx)
    return await query_hybrid_search(db, query, ...)
```

**Web UI usage** (direct, no adapter needed):
```python
# memcp/webui/main.py
async with get_db_connection() as db:
    results = await query_hybrid_search(db, "python", embedding, [], 10)
```

This makes all `query_*` functions reusable from both MCP and web UI without any adapter.

### 2. New DB Queries to Add
Add to `memcp/db.py`:
- `query_recent_activity(limit)` - entities sorted by `accessed DESC`
- `query_full_graph(limit)` - all entities + relations for viz
- `query_health_stats(stale_days)` - total/stale/low_confidence counts
- `query_all_relations()` - for graph visualization

### 3. Graph Visualization
Embed Cytoscape.js via `ui.html()` with JSON data from `query_full_graph()`:
- Force-directed layout (`cose`)
- Node colors by entity type
- Click events → Python callbacks via JavaScript interop

### 4. Styling (match mock exactly)
Dark theme with cyan accents:
- Background: `#111827` (gray-900)
- Cards: `#1f2937` (gray-800) with `border-cyan-500/30`
- Primary: `#06b6d4` (cyan-500)
- Text: `#f9fafb` (gray-50)

### 5. Docker Configuration
```dockerfile
FROM python:3.13-slim
COPY memcp/ ./memcp/
RUN pip install nicegui plotly
CMD ["python", "-m", "memcp.webui.main"]
EXPOSE 8080
```

docker-compose.yml adds `webui` service alongside existing `surrealdb`.

## Implementation Order

### Phase 0: DB Refactor
1. Refactor `run_query()` to accept `db: AsyncSurreal` instead of `ctx: Context`
2. Update all `query_*` functions to pass `db` to `run_query()`
3. Update MCP servers (`servers/*.py`) to extract db and pass it
4. Run tests to verify no regressions

### Phase 1: Foundation
5. Create `memcp/webui/` package
6. Create `main.py` with NiceGUI app skeleton + db connection management
7. Add dark theme styling in `theme.py`
8. Implement sidebar navigation component

### Phase 2: Core Views
9. Dashboard page (stat cards, Plotly charts)
10. Search page (hybrid search, filters, result cards)
11. Entity detail page (content, metadata, relations)

### Phase 3: Graph View
12. Cytoscape.js integration
13. Add `query_full_graph()` to db.py
14. Filter sidebar + node click interactions

### Phase 4: Write Operations
15. Add Memory form with validation
16. Relation builder component
17. File Upload (text extraction via PyMuPDF)

### Phase 5: Maintenance
18. Reflect configuration panel
19. Conflict resolution cards
20. Health donut chart (Plotly)

### Phase 6: Polish & Deploy
21. Loading states, error handling
22. Docker multi-stage build
23. Update README with usage docs

## Files to Modify
- `memcp/db.py` - Refactor to accept `db: AsyncSurreal` instead of `ctx: Context`; add 4 new query functions
- `memcp/servers/*.py` - Update to pass `db` from context to query functions
- `pyproject.toml` - Add dependencies: `nicegui`, `plotly`, `pymupdf`

## Files to Create
- `memcp/webui/__init__.py`
- `memcp/webui/main.py`
- `memcp/webui/theme.py`
- `memcp/webui/pages/*.py` (7 files)
- `memcp/webui/components/*.py` (4 files)
- `Dockerfile.webui`
- Update `docker-compose.yml`

## Notes
- **No auth initially** - Can add later if needed
- **Semantic weight slider** - Search page; maps to RRF weight adjustment in `query_hybrid_search`
- **Auto-tagging** - Omitted for now; can add LLM integration later if needed
