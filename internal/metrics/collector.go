// Package metrics provides Prometheus metrics for the Know server.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for the application.
// All methods are safe to call on a nil receiver (no-op).
type Metrics struct {
	// Infrastructure
	dbDuration           *prometheus.HistogramVec
	httpDuration         *prometheus.HistogramVec
	httpRequestsTotal    *prometheus.CounterVec
	embedDuration        *prometheus.HistogramVec
	llmDuration          *prometheus.HistogramVec
	llmTokens            *prometheus.CounterVec
	pipelineDuration     *prometheus.HistogramVec
	pipelineTotal        *prometheus.CounterVec
	authEventsTotal      *prometheus.CounterVec
	rateLimitRejectTotal *prometheus.CounterVec
	cleanupFailuresTotal *prometheus.CounterVec

	// Agent tools
	toolCallsTotal *prometheus.CounterVec
	toolDuration   *prometheus.HistogramVec

	// Memory system
	memoryTotal    *prometheus.CounterVec
	memoryDuration *prometheus.HistogramVec

	// Search
	searchDuration      *prometheus.HistogramVec
	searchTotal         *prometheus.CounterVec
	embeddingCacheTotal *prometheus.CounterVec

	// WebDAV
	webdavTotal    *prometheus.CounterVec
	webdavDuration *prometheus.HistogramVec

	// SSH/SFTP
	sshConnectionsTotal *prometheus.CounterVec
	sftpTotal           *prometheus.CounterVec
	sftpDuration        *prometheus.HistogramVec

	// Agent conversations
	agentDuration    prometheus.Histogram
	agentToolCount   prometheus.Histogram
	agentTokensTotal *prometheus.CounterVec

	// Web clipping
	webclipDuration *prometheus.HistogramVec
	webclipTotal    *prometheus.CounterVec

	// Remote federation
	remoteToolDuration    *prometheus.HistogramVec
	remoteToolTotal       *prometheus.CounterVec
	remoteVaultCacheTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		// ── Infrastructure ──────────────────────────────────────────

		dbDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_db_operation_duration_seconds",
			Help:    "Duration of database operations in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"operation"}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),

		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path", "status"}),

		embedDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_embedding_duration_seconds",
			Help:    "Duration of embedding operations in seconds.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"model"}),

		llmDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_llm_operation_duration_seconds",
			Help:    "Duration of LLM operations in seconds.",
			Buckets: []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"operation", "model"}),

		llmTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_llm_tokens_total",
			Help: "Total number of LLM tokens processed.",
		}, []string{"operation", "model", "direction"}),

		pipelineDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_pipeline_job_duration_seconds",
			Help:    "Duration of pipeline jobs in seconds.",
			Buckets: []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"type"}),

		pipelineTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_pipeline_jobs_total",
			Help: "Total number of pipeline jobs by type and status.",
		}, []string{"type", "status"}),

		authEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_auth_events_total",
			Help: "Total number of authentication events by event type and result.",
		}, []string{"event", "result"}),

		rateLimitRejectTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_rate_limit_rejected_total",
			Help: "Total number of requests rejected by rate limiting.",
		}, []string{"tier"}),

		cleanupFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_cleanup_failures_total",
			Help: "Total number of background cleanup task failures.",
		}, []string{"task"}),

		// ── Agent tools ─────────────────────────────────────────────

		toolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_tool_calls_total",
			Help: "Total tool calls by tool name and status.",
		}, []string{"tool", "status"}),

		toolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_tool_duration_seconds",
			Help:    "Duration of tool executions in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"tool"}),

		// ── Memory system ───────────────────────────────────────────

		memoryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_memory_total",
			Help: "Total memory operations by type and status.",
		}, []string{"operation", "status"}),

		memoryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_memory_duration_seconds",
			Help:    "Duration of memory operations in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"operation"}),

		// ── Search ──────────────────────────────────────────────────

		searchDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_search_duration_seconds",
			Help:    "Duration of search operations in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"strategy"}),

		searchTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_search_total",
			Help: "Total search operations by strategy (bm25, hybrid, fallback).",
		}, []string{"strategy"}),

		embeddingCacheTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_embedding_cache_total",
			Help: "Embedding cache lookups by result.",
		}, []string{"result"}),

		// ── WebDAV ──────────────────────────────────────────────────

		webdavTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_webdav_total",
			Help: "Total WebDAV operations by HTTP method.",
		}, []string{"method"}),

		webdavDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_webdav_duration_seconds",
			Help:    "Duration of WebDAV operations in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),

		// ── SSH/SFTP ────────────────────────────────────────────────

		sshConnectionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_ssh_connections_total",
			Help: "Total SSH connection attempts by result.",
		}, []string{"result"}),

		sftpTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_sftp_total",
			Help: "Total SFTP operations by type.",
		}, []string{"operation"}),

		sftpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_sftp_duration_seconds",
			Help:    "Duration of SFTP operations in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"operation"}),

		// ── Agent conversations ─────────────────────────────────────

		agentDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "know_agent_duration_seconds",
			Help:    "Duration of agent chat loops in seconds.",
			Buckets: []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		}),

		agentToolCount: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "know_agent_tool_count",
			Help:    "Number of tools called per agent conversation turn.",
			Buckets: []float64{0, 1, 2, 3, 5, 8, 13, 21},
		}),

		agentTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_agent_tokens_total",
			Help: "Total tokens consumed by agent conversations.",
		}, []string{"direction"}),

		// ── Web clipping ────────────────────────────────────────────

		webclipDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_webclip_duration_seconds",
			Help:    "Duration of web clip fetch operations in seconds.",
			Buckets: []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"llm_cleanup"}),

		webclipTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_webclip_total",
			Help: "Total web clip operations by status.",
		}, []string{"status"}),

		// ── Remote federation ───────────────────────────────────────

		remoteToolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "know_remote_tool_duration_seconds",
			Help:    "Duration of remote tool executions in seconds.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"tool"}),

		remoteToolTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_remote_tool_total",
			Help: "Total remote tool executions by tool and status.",
		}, []string{"tool", "status"}),

		remoteVaultCacheTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "know_remote_vault_cache_total",
			Help: "Remote vault cache lookups by result.",
		}, []string{"result"}),
	}

	prometheus.MustRegister(
		// Infrastructure
		m.dbDuration,
		m.httpDuration,
		m.httpRequestsTotal,
		m.embedDuration,
		m.llmDuration,
		m.llmTokens,
		m.pipelineDuration,
		m.pipelineTotal,
		m.authEventsTotal,
		m.rateLimitRejectTotal,
		m.cleanupFailuresTotal,
		// Agent tools
		m.toolCallsTotal,
		m.toolDuration,
		// Memory
		m.memoryTotal,
		m.memoryDuration,
		// Search
		m.searchDuration,
		m.searchTotal,
		m.embeddingCacheTotal,
		// WebDAV
		m.webdavTotal,
		m.webdavDuration,
		// SSH/SFTP
		m.sshConnectionsTotal,
		m.sftpTotal,
		m.sftpDuration,
		// Agent conversations
		m.agentDuration,
		m.agentToolCount,
		m.agentTokensTotal,
		// Web clipping
		m.webclipDuration,
		m.webclipTotal,
		// Remote federation
		m.remoteToolDuration,
		m.remoteToolTotal,
		m.remoteVaultCacheTotal,
	)

	return m
}

// ── Infrastructure ──────────────────────────────────────────────────────

// RecordTiming records a DB operation duration. The op parameter should include
// the table prefix, e.g. "db.file.create".
func (m *Metrics) RecordTiming(op string, duration time.Duration) {
	if m == nil {
		return
	}
	m.dbDuration.WithLabelValues(op).Observe(duration.Seconds())
}

// RecordLLMUsage records LLM operation duration and token counts.
func (m *Metrics) RecordLLMUsage(op string, model string, duration time.Duration, inputTokens, outputTokens int64) {
	if m == nil {
		return
	}
	m.llmDuration.WithLabelValues(op, model).Observe(duration.Seconds())
	m.llmTokens.WithLabelValues(op, model, "input").Add(float64(inputTokens))
	m.llmTokens.WithLabelValues(op, model, "output").Add(float64(outputTokens))
}

// RecordEmbedding records an embedding operation duration.
func (m *Metrics) RecordEmbedding(model string, duration time.Duration) {
	if m == nil {
		return
	}
	m.embedDuration.WithLabelValues(model).Observe(duration.Seconds())
}

// RecordHTTPRequest records an HTTP request's duration and increments the request counter.
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	s := strconv.Itoa(status)
	m.httpDuration.WithLabelValues(method, path, s).Observe(duration.Seconds())
	m.httpRequestsTotal.WithLabelValues(method, path, s).Inc()
}

// RecordAuthEvent records an authentication event (success, failure, expired).
func (m *Metrics) RecordAuthEvent(event, result string) {
	if m == nil {
		return
	}
	m.authEventsTotal.WithLabelValues(event, result).Inc()
}

// RecordRateLimitRejection records a rate-limited request rejection.
func (m *Metrics) RecordRateLimitRejection(tier string) {
	if m == nil {
		return
	}
	m.rateLimitRejectTotal.WithLabelValues(tier).Inc()
}

// RecordCleanupFailure records a background cleanup task failure.
func (m *Metrics) RecordCleanupFailure(task string) {
	if m == nil {
		return
	}
	m.cleanupFailuresTotal.WithLabelValues(task).Inc()
}

// RecordPipelineJob records a pipeline job completion.
func (m *Metrics) RecordPipelineJob(jobType, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.pipelineDuration.WithLabelValues(jobType).Observe(duration.Seconds())
	m.pipelineTotal.WithLabelValues(jobType, status).Inc()
}

// ── Agent tools ─────────────────────────────────────────────────────────

// RecordToolCall records a tool execution's duration and increments the call counter.
func (m *Metrics) RecordToolCall(tool, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.toolDuration.WithLabelValues(tool).Observe(duration.Seconds())
	m.toolCallsTotal.WithLabelValues(tool, status).Inc()
}

// ── Memory system ───────────────────────────────────────────────────────

// RecordMemoryOp records a memory operation's duration and increments the operation counter.
func (m *Metrics) RecordMemoryOp(operation, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.memoryDuration.WithLabelValues(operation).Observe(duration.Seconds())
	m.memoryTotal.WithLabelValues(operation, status).Inc()
}

// ── Search ──────────────────────────────────────────────────────────────

// RecordSearch records a search operation's duration and strategy used.
// strategy should be "bm25", "hybrid", or "fallback".
func (m *Metrics) RecordSearch(strategy string, duration time.Duration) {
	if m == nil {
		return
	}
	m.searchDuration.WithLabelValues(strategy).Observe(duration.Seconds())
	m.searchTotal.WithLabelValues(strategy).Inc()
}

// RecordEmbeddingCache records an embedding cache lookup result ("hit" or "miss").
func (m *Metrics) RecordEmbeddingCache(result string) {
	if m == nil {
		return
	}
	m.embeddingCacheTotal.WithLabelValues(result).Inc()
}

// ── WebDAV ──────────────────────────────────────────────────────────────

// RecordWebDAVOp records a WebDAV operation's duration by HTTP method.
func (m *Metrics) RecordWebDAVOp(method string, duration time.Duration) {
	if m == nil {
		return
	}
	m.webdavDuration.WithLabelValues(method).Observe(duration.Seconds())
	m.webdavTotal.WithLabelValues(method).Inc()
}

// ── SSH/SFTP ────────────────────────────────────────────────────────────

// RecordSSHConnection records an SSH connection attempt result ("success" or "failure").
func (m *Metrics) RecordSSHConnection(result string) {
	if m == nil {
		return
	}
	m.sshConnectionsTotal.WithLabelValues(result).Inc()
}

// RecordSFTPOp records an SFTP operation's duration by operation type.
func (m *Metrics) RecordSFTPOp(operation string, duration time.Duration) {
	if m == nil {
		return
	}
	m.sftpDuration.WithLabelValues(operation).Observe(duration.Seconds())
	m.sftpTotal.WithLabelValues(operation).Inc()
}

// ── Agent conversations ─────────────────────────────────────────────────

// RecordAgentChat records an agent chat turn's duration, tool count, and token usage.
func (m *Metrics) RecordAgentChat(duration time.Duration, toolCount int, inputTokens, outputTokens int64) {
	if m == nil {
		return
	}
	m.agentDuration.Observe(duration.Seconds())
	m.agentToolCount.Observe(float64(toolCount))
	m.agentTokensTotal.WithLabelValues("input").Add(float64(inputTokens))
	m.agentTokensTotal.WithLabelValues("output").Add(float64(outputTokens))
}

// ── Web clipping ────────────────────────────────────────────────────────

// RecordWebClip records a web clip operation's duration and status.
// llmCleanup indicates whether LLM-based markdown cleanup was applied.
func (m *Metrics) RecordWebClip(llmCleanup bool, status string, duration time.Duration) {
	if m == nil {
		return
	}
	label := "false"
	if llmCleanup {
		label = "true"
	}
	m.webclipDuration.WithLabelValues(label).Observe(duration.Seconds())
	m.webclipTotal.WithLabelValues(status).Inc()
}

// ── Remote federation ───────────────────────────────────────────────────

// RecordRemoteTool records a remote tool execution's duration and increments the call counter.
func (m *Metrics) RecordRemoteTool(tool, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.remoteToolDuration.WithLabelValues(tool).Observe(duration.Seconds())
	m.remoteToolTotal.WithLabelValues(tool, status).Inc()
}

// RecordRemoteVaultCache records a remote vault cache lookup result ("hit" or "miss").
func (m *Metrics) RecordRemoteVaultCache(result string) {
	if m == nil {
		return
	}
	m.remoteVaultCacheTotal.WithLabelValues(result).Inc()
}
