# API Documentation — Architecture

Technical reference for how the interactive API docs are built and served. For accessing and using the docs, see [feature-api-docs.md](feature-api-docs.md).

## Architecture

- **OpenAPI spec**: Hand-written YAML at `internal/api/openapi.yaml`, embedded in the binary via `go:embed`
- **Scalar UI**: Loaded from CDN (`cdn.jsdelivr.net/npm/@scalar/api-reference`), no bundled JS
- **Routes**: `GET /` (Scalar UI) and `GET /api/v1/openapi.yaml` (spec) — both unauthenticated
- **Registration**: `api.RegisterDocs(mux)` in `cmd/know/cmd_serve.go`
- **Tag groups**: `x-tagGroups` extension in the spec controls sidebar organization
