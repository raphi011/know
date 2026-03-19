package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

//go:embed docs.html
var docsHTML []byte

// RegisterDocs registers the API documentation endpoints (unauthenticated).
// GET /              — Scalar API reference UI
// GET /api/v1/openapi.yaml — OpenAPI 3.1 spec
func RegisterDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(openapiSpec) // client disconnected — nothing to do
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(docsHTML) // client disconnected — nothing to do
	})
}
