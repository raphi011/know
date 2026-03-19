package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestEmbeddedOpenAPISpecIsValidYAML(t *testing.T) {
	if len(openapiSpec) == 0 {
		t.Fatal("embedded openapi.yaml is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(openapiSpec, &doc); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}
	if _, ok := doc["openapi"]; !ok {
		t.Fatal("openapi.yaml missing 'openapi' version field")
	}
	if _, ok := doc["paths"]; !ok {
		t.Fatal("openapi.yaml missing 'paths' field")
	}
}

func TestDocsEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	RegisterDocs(mux)

	t.Run("GET /api/v1/openapi.yaml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if ct != "application/yaml" {
			t.Fatalf("expected Content-Type application/yaml, got %q", ct)
		}
		if rec.Body.Len() == 0 {
			t.Fatal("response body is empty")
		}
	})

	t.Run("GET /", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Fatalf("expected Content-Type text/html; charset=utf-8, got %q", ct)
		}
		body := rec.Body.String()
		if len(body) == 0 {
			t.Fatal("response body is empty")
		}
		if !strings.Contains(body, "scalar") && !strings.Contains(body, "Scalar") {
			t.Fatal("HTML does not reference Scalar")
		}
	})
}
