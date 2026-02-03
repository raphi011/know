# MCP SurrealDB Server

An MCP (Model Context Protocol) server in Go that connects to a SurrealDB instance to persist knowledge between agent sessions.

## Purpose

This server enables AI agents to store and retrieve knowledge across sessions, providing a persistent memory layer using SurrealDB as the backend database.

## Tech Stack

- **Language**: Go
- **Protocol**: MCP (Model Context Protocol)
- **Database**: SurrealDB

## Development Workflow

**IMPORTANT**: Before committing any changes, always run tests:

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/db/...
```

## Building

```bash
# Build the binary
go build -o memcp ./cmd/memcp

# Run the server
./memcp
```

## SurrealDB Reference

For SurrealDB-specific syntax, v3.0 breaking changes, and query patterns:
- **Subagent**: Use the `surrealdb` subagent for complex query work (has built-in reference guide)

## Documentation

**IMPORTANT**: When adding or modifying features, always update `README.md` with example prompts showcasing what the feature can do. This helps users understand how to use each tool effectively.
