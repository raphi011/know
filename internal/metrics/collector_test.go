package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNilMetrics(t *testing.T) {
	// All Record* methods must be safe to call on a nil receiver.
	var m *Metrics

	m.RecordTiming("foo.get", time.Second)
	m.RecordLLMUsage("chat", "gpt-4", time.Second, 100, 50)
	m.RecordEmbedding("model", time.Second)
	m.RecordHTTPRequest("GET", "/api/docs", 200, time.Second)
	m.RecordAuthEvent("login", "success")
	m.RecordRateLimitRejection("free")
	m.RecordCleanupFailure("orphans")
	m.RecordPipelineJob("embed", "success", time.Second)
	m.RecordToolCall("search", "success", time.Second)
	m.RecordMemoryOp("create", "success", time.Second)
	m.RecordSearch("hybrid", time.Second)
	m.RecordEmbeddingCache("hit")
	m.RecordWebDAVOp("GET", time.Second)
	m.RecordSSHConnection("success")
	m.RecordSFTPOp("read", time.Second)
	m.RecordAgentChat(time.Second, 3, 100, 50)
	m.RecordWebClip(true, "success", time.Second)
	m.RecordRemoteTool("search", "success", time.Second)
	m.RecordRemoteVaultCache("hit")
}

func TestNewMetrics(t *testing.T) {
	// NewMetrics must register all metrics without panicking.
	// Use a custom registry to avoid conflicts with the global one.
	orig := prometheus.DefaultRegisterer
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	t.Cleanup(func() { prometheus.DefaultRegisterer = orig })

	m := NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	// Verify metrics can actually be observed (catches label cardinality mismatches).
	m.RecordToolCall("search", "success", time.Millisecond)
	m.RecordMemoryOp("create", "error", time.Millisecond)
	m.RecordSearch("bm25", time.Millisecond)
	m.RecordEmbeddingCache("miss")
	m.RecordWebDAVOp("PUT", time.Millisecond)
	m.RecordSSHConnection("failure")
	m.RecordSFTPOp("write", time.Millisecond)
	m.RecordAgentChat(time.Second, 5, 200, 100)
	m.RecordWebClip(false, "error", time.Millisecond)
	m.RecordRemoteTool("read_document", "success", time.Millisecond)
	m.RecordRemoteVaultCache("miss")
}
