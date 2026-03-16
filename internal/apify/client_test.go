package apify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunActorSync_Success(t *testing.T) {
	items := []map[string]string{
		{"title": "Test Video", "transcript": "Hello world"},
	}
	itemsJSON, _ := json.Marshal(items)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write(itemsJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	// Use a custom RoundTripper to redirect requests to the test server.
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	result, err := client.RunActorSync(context.Background(), "test/actor", map[string]string{"url": "https://youtube.com/watch?v=abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
}

func TestRunActorSync_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte(`{"error":"Actor not found"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	_, err := client.RunActorSync(context.Background(), "nonexistent/actor", nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestRunActorSync_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	_, err := client.RunActorSync(context.Background(), "test/actor", nil)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
}

func TestRunActorSync_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate slow response — context should cancel first.
		select {}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.RunActorSync(ctx, "test/actor", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// rewriteTransport redirects all requests to a test server URL while
// preserving the original path and query parameters.
type rewriteTransport struct {
	target string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	// Parse target to get host.
	req.URL.Host = t.target[len("http://"):]
	return http.DefaultTransport.RoundTrip(req)
}
