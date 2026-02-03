# SurrealDB Go SDK v3 Compatibility Issues

## Graph Traversal CBOR Range Type Issue

**Status**: KNOWN INCOMPATIBILITY - Tests skipped until SDK fix

**Affected Functions**:
- `QueryTraverse()` - Bidirectional graph traversal with depth limits
- `QueryFindPath()` - Shortest path finding between entities

**Root Cause**:
The Go SDK v1.2.0 cannot decode SurrealDB v3 graph traversal results that use range syntax:
- `->relates..{depth}->entity` 
- `->relates..1->entity`
- `->relates..5->entity`

SurrealDB v3 returns range bounds using new CBOR types that the SDK doesn't handle, causing decode panics.

**Error Symptoms**:
```
panic: runtime error: invalid memory address or nil pointer dereference
CBOR decode error with range types
```

**Workaround**:
Currently NO WORKAROUND available. The query pattern is fundamental to graph traversal.

Possible alternatives (not yet tested):
1. Manual iterative queries (depth-first or breadth-first)
2. Use raw Query() and parse results manually
3. Wait for SDK update

**Tests**:
- `TestQueryTraverse` - SKIPPED with clear message
- `TestQueryFindPath` - SKIPPED with clear message

Both tests remain in codebase for when SDK is fixed.

**Documentation**:
- Added WARNING comments to QueryTraverse() and QueryFindPath() functions
- Skip messages explain the issue clearly

**Tracking**:
- No open issue found in surrealdb/surrealdb.go repo yet
- Consider opening issue if needed

**Next Steps**:
1. Monitor SDK releases for CBOR range type support
2. Re-enable tests when SDK is updated
3. Consider filing SDK issue if not already tracked

## Other v3 Compatibility Notes

### Set Type (CBOR Tag 56)
**Status**: WORKAROUND IMPLEMENTED

Used `array<string>` instead of `set<string>` for labels fields. This is documented in schema and will be migrated when SDK supports CBOR tag 56.

### Record ID Handling
**Status**: WORKING

v3 changed record IDs to use CBOR record type. SDK handles this correctly with proper parameterization using `type::record()` in queries.

### Cast Syntax
**Status**: WORKING

v3 requires explicit casts for type conversions (e.g., `<string>id` for record ID to string). All queries updated.

### Graph Edge Tables
**Status**: WORKING

Relation tables (`relates`, `extracted_from`) work correctly with TYPE RELATION syntax.
