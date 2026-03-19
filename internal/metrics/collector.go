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
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
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
	}

	prometheus.MustRegister(
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
	)

	return m
}

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
