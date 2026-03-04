package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTavilySearch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		var req tavilyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.APIKey != "test-key" {
			t.Errorf("expected api key test-key, got %s", req.APIKey)
		}
		if req.Query != "what is Go" {
			t.Errorf("expected query 'what is Go', got %s", req.Query)
		}
		if req.MaxResults != 5 {
			t.Errorf("expected max_results 5, got %d", req.MaxResults)
		}

		resp := tavilyResponse{
			Answer: "Go is a programming language.",
			Results: []tavilyResult{
				{Title: "Go Website", URL: "https://go.dev", Content: "The Go programming language"},
				{Title: "Go Wiki", URL: "https://en.wikipedia.org/wiki/Go", Content: "Go is a statically typed language"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &tavilyClient{
		apiKey:     "test-key",
		httpClient: srv.Client(),
	}
	// Override the URL by using a custom transport
	client.httpClient.Transport = rewriteURLTransport{url: srv.URL, base: srv.Client().Transport}

	result, err := client.Search(context.Background(), "what is Go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "**Summary:** Go is a programming language.") {
		t.Errorf("expected summary in result, got: %s", result)
	}
	if !strings.Contains(result, "### [Go Website](https://go.dev)") {
		t.Errorf("expected Go Website result, got: %s", result)
	}
	if !strings.Contains(result, "### [Go Wiki](https://en.wikipedia.org/wiki/Go)") {
		t.Errorf("expected Go Wiki result, got: %s", result)
	}
}

func TestTavilySearch_AnswerOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyResponse{Answer: "Just an answer."}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient("key", srv)
	result, err := client.Search(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "**Summary:** Just an answer.\n\n" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestTavilySearch_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyResponse{}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient("key", srv)
	result, err := client.Search(context.Background(), "obscure query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "No web results found." {
		t.Errorf("expected fallback message, got: %q", result)
	}
}

func TestTavilySearch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Invalid API key"}`))
	}))
	defer srv.Close()

	client := newTestClient("bad-key", srv)
	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("expected error body in message, got: %v", err)
	}
}

func TestTavilySearch_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyResponse{Answer: "should not reach"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient("key", srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.Search(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestTavilySearch_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := newTestClient("key", srv)
	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestTavilySearch_Integration(t *testing.T) {
	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		t.Skip("TAVILY_API_KEY not set, skipping integration test")
	}

	client := newTavilyClient(apiKey)
	result, err := client.Search(context.Background(), "what is the Go programming language")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result == "" || result == "No web results found." {
		t.Error("expected non-empty results from real API")
	}
	if !strings.Contains(result, "**Summary:**") {
		t.Error("expected summary in response")
	}
	if !strings.Contains(result, "###") {
		t.Error("expected at least one result heading")
	}

	t.Logf("Result preview: %.300s...", result)
}

// rewriteURLTransport redirects all requests to the test server URL.
type rewriteURLTransport struct {
	url  string
	base http.RoundTripper
}

func (t rewriteURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.url, "http://")
	return t.base.RoundTrip(req)
}

func newTestClient(apiKey string, srv *httptest.Server) *tavilyClient {
	return &tavilyClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Transport: rewriteURLTransport{url: srv.URL, base: srv.Client().Transport},
		},
	}
}
