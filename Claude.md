# MCP SurrealDB Server

An MCP (Model Context Protocol) server in Python that connects to a SurrealDB instance to persist knowledge between agent sessions.

## Purpose

This server enables AI agents to store and retrieve knowledge across sessions, providing a persistent memory layer using SurrealDB as the backend database.

## Tech Stack

- **Language**: Python
- **Protocol**: MCP (Model Context Protocol)
- **Database**: SurrealDB

## Features (Planned)

- Connect to a SurrealDB instance
- Store knowledge/memories from agent sessions
- Retrieve relevant knowledge for new sessions
- Query and search stored information
- Manage knowledge lifecycle (create, read, update, delete)

## Development Workflow

**IMPORTANT**: After making any changes to the codebase, always run the build verification test:

```bash
uv run pytest test_memcp.py -v
```

This ensures the module compiles correctly and can be imported without errors.

## SurrealDB Syntax Learnings

Key syntax rules and gotchas when working with SurrealDB queries:

### Record ID Formatting

**RELATE statements must use direct record ID syntax:**
```surql
✅ CORRECT: RELATE entity:from_id->rel_type->entity:to_id SET weight = $weight
❌ WRONG:   RELATE type::thing("entity", $from)->$type->type::thing("entity", $to) SET weight = $weight
```
Reason: SurrealDB doesn't support `type::thing()` inside RELATE statements. Use string interpolation for IDs and relation types.

**Record ID comparisons need type::thing():**
```surql
✅ CORRECT: WHERE id != type::thing("entity", $id)
❌ WRONG:   WHERE id != $id  (won't match record IDs properly)
```

### KNN Vector Search Operator

**The KNN operator <|...|> has limitations:**
```surql
❌ NOT WORKING: WHERE embedding <|$limit,100|> $emb  (parameters not supported)
❌ NOT RELIABLE: WHERE embedding <|5,100|> $emb     (may return empty even with valid embeddings)
```

**Use vector::similarity::cosine instead:**
```surql
✅ RECOMMENDED:
SELECT id, content, vector::similarity::cosine(embedding, $emb) AS sim
FROM entity
ORDER BY sim DESC
LIMIT $limit
```
Reason: The KNN operator requires MTREE index which may not work in all environments. The similarity function is more reliable and works without the index.

### Parameter Usage

**Literal integers required in KNN operators:**
```surql
❌ WRONG:   <|$limit,100|>   (parameters not allowed)
✅ CORRECT: <|{limit},100|>  (f-string interpolation)
```

**Limit can be parameterized in LIMIT clause:**
```surql
✅ CORRECT: LIMIT $limit  (parameters work here)
```

### DELETE Operations

**Simple deletes are preferred:**
```surql
✅ CORRECT: DELETE type::thing("entity", $id)
❌ COMPLEX: DELETE FROM type::thing("entity", $id)->?;  (may cause parse errors)
```
Reason: SurrealDB automatically cleans up relations when a record is deleted. Extra DELETE statements for relations are usually unnecessary.

