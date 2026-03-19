# API Documentation (Scalar)

Interactive API reference for the Know REST API, powered by [Scalar](https://github.com/scalar/scalar).

## Access

Open `http://localhost:<port>/` in your browser (default: `http://localhost:8484/`).

The raw OpenAPI 3.1 spec is available at `/api/v1/openapi.yaml`.

## Features

- Browse all REST API endpoints grouped by resource
- View request/response schemas with examples
- Try API calls directly in the browser (enter your Bearer token in the auth section)
- Dark mode by default

## Architecture

- **OpenAPI spec**: Hand-written YAML at `internal/api/openapi.yaml`, embedded in the binary via `go:embed`
- **Scalar UI**: Loaded from CDN (`cdn.jsdelivr.net/npm/@scalar/api-reference`), no bundled JS
- **Routes**: `GET /` (Scalar UI) and `GET /api/v1/openapi.yaml` (spec) — both unauthenticated
- **Registration**: `api.RegisterDocs(mux)` in `cmd/know/cmd_serve.go`

## Maintaining the Spec

When adding, modifying, or removing REST API endpoints, update `internal/api/openapi.yaml`:

1. Add/update the path under `paths:`
2. Add any new request/response schemas under `components.schemas:`
3. Run `just build` to verify the embedded spec compiles
4. Open `http://localhost:8484/` to verify it renders correctly

## Example Prompts

- "Show me the API docs" → Open `http://localhost:8484/`
- "What endpoints does Know have?" → Browse the Scalar UI or read `internal/api/openapi.yaml`
- "Try searching for documents" → Use the Scalar "Try it" feature on `GET /vaults/{vault}/search`
