# API Documentation (Scalar)

Interactive API reference for the Know REST API, powered by [Scalar](https://github.com/scalar/scalar).

## Access

Open `http://localhost:<port>/` in your browser (default: `http://localhost:8484/`).

The raw OpenAPI 3.1 spec is available at `/api/v1/openapi.yaml`.

## Features

- **Sidebar grouping**: Endpoints organized into logical sections (Authentication, Knowledge Base, Content Management, AI Agent, Automation & Sync, Administration)
- **Getting started guide**: Inline quick-start documentation covering auth, core workflows, and common patterns
- **Rich schema descriptions**: Every field has a description with examples, making "Try It" useful out of the box
- **Search**: Press `Cmd+K` (or `Ctrl+K`) to search endpoints, schemas, and descriptions
- **Dark mode** by default
- **BearerAuth** pre-selected in the auth section

## Technical Reference

For architecture details (OpenAPI embedding, Scalar CDN loading, route registration), see [tech-api-docs.md](tech-api-docs.md).

## Maintaining the Spec

When adding, modifying, or removing REST API endpoints, update `internal/api/openapi.yaml`:

1. Add/update the path under `paths:`
2. Add any new request/response schemas under `components.schemas:` with descriptions and examples
3. Ensure new tags are added to the appropriate `x-tagGroups` section
4. Run `just build` to verify the embedded spec compiles
5. Open `http://localhost:8484/` to verify it renders correctly

### Spec Quality Checklist

- Every schema field has a `description`
- Important fields have `example` values (paths, names, scores, etc.)
- Tag descriptions explain what the resource group does (2-4 sentences)
- Request/response schemas for key endpoints have realistic examples

## Example Prompts

- "Show me the API docs" → Open `http://localhost:8484/`
- "What endpoints does Know have?" → Browse the Scalar UI or read `internal/api/openapi.yaml`
- "Try searching for documents" → Use the Scalar "Try it" feature on `GET /vaults/{vault}/search`
- "How does auth work?" → Read the Getting Started section in the API docs
