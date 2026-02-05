// Package metrics provides in-memory runtime statistics collection.
package metrics

import (
	"math"
	"sync"
	"time"
)

// OperationMetrics holds aggregated metrics for a single operation type.
type OperationMetrics struct {
	Count     int64
	TotalTime time.Duration
	MinTime   time.Duration
	MaxTime   time.Duration

	// Token metrics (only for LLM operations)
	TotalInputTokens  int64
	TotalOutputTokens int64
	MinInputTokens    int64
	MaxInputTokens    int64
	MinOutputTokens   int64
	MaxOutputTokens   int64
}

// OperationSnapshot provides computed stats from raw metrics.
type OperationSnapshot struct {
	Count       int64
	TotalTimeMs int64
	AvgTimeMs   float64
	MinTimeMs   int64
	MaxTimeMs   int64

	// Token stats (nil if not applicable)
	TotalInputTokens  *int64
	TotalOutputTokens *int64
	AvgInputTokens    *float64
	AvgOutputTokens   *float64
	MinInputTokens    *int64
	MaxInputTokens    *int64
	MinOutputTokens   *int64
	MaxOutputTokens   *int64
}

// Snapshot represents the full server statistics at a point in time.
type Snapshot struct {
	UptimeSeconds float64
	Embedding     *OperationSnapshot
	LLMGenerate   *OperationSnapshot
	LLMStream     *OperationSnapshot
	DBQuery       *OperationSnapshot
	DBSearch      *OperationSnapshot
}

// Operation names for the collector.
const (
	OpEmbedding   = "embedding"
	OpLLMGenerate = "llm_generate"
	OpLLMStream   = "llm_stream"
	OpDBQuery     = "db_query"
	OpDBSearch    = "db_search"
)

// Collector aggregates in-memory runtime statistics.
// All methods are thread-safe.
type Collector struct {
	mu        sync.RWMutex
	startTime time.Time
	ops       map[string]*OperationMetrics
}

// NewCollector creates a new metrics collector.
func NewCollector() *Collector {
	return &Collector{
		startTime: time.Now(),
		ops:       make(map[string]*OperationMetrics),
	}
}

// getOrCreate returns existing metrics or creates new ones for an operation.
// Caller must hold write lock.
func (c *Collector) getOrCreate(op string) *OperationMetrics {
	m, ok := c.ops[op]
	if !ok {
		m = &OperationMetrics{
			MinTime:         time.Duration(math.MaxInt64),
			MinInputTokens:  math.MaxInt64,
			MinOutputTokens: math.MaxInt64,
		}
		c.ops[op] = m
	}
	return m
}

// RecordTiming records timing for an operation.
func (c *Collector) RecordTiming(op string, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	m := c.getOrCreate(op)
	m.Count++
	m.TotalTime += duration

	if duration < m.MinTime {
		m.MinTime = duration
	}
	if duration > m.MaxTime {
		m.MaxTime = duration
	}
}

// RecordLLMUsage records timing and token usage for an LLM operation.
func (c *Collector) RecordLLMUsage(op string, duration time.Duration, inputTokens, outputTokens int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	m := c.getOrCreate(op)
	m.Count++
	m.TotalTime += duration

	if duration < m.MinTime {
		m.MinTime = duration
	}
	if duration > m.MaxTime {
		m.MaxTime = duration
	}

	m.TotalInputTokens += inputTokens
	m.TotalOutputTokens += outputTokens

	if inputTokens < m.MinInputTokens {
		m.MinInputTokens = inputTokens
	}
	if inputTokens > m.MaxInputTokens {
		m.MaxInputTokens = inputTokens
	}
	if outputTokens < m.MinOutputTokens {
		m.MinOutputTokens = outputTokens
	}
	if outputTokens > m.MaxOutputTokens {
		m.MaxOutputTokens = outputTokens
	}
}

// snapshotOp creates a snapshot for an operation, returning nil if no data.
func snapshotOp(m *OperationMetrics, includeTokens bool) *OperationSnapshot {
	if m == nil || m.Count == 0 {
		return nil
	}

	snap := &OperationSnapshot{
		Count:       m.Count,
		TotalTimeMs: m.TotalTime.Milliseconds(),
		AvgTimeMs:   float64(m.TotalTime.Milliseconds()) / float64(m.Count),
		MinTimeMs:   m.MinTime.Milliseconds(),
		MaxTimeMs:   m.MaxTime.Milliseconds(),
	}

	if includeTokens && (m.TotalInputTokens > 0 || m.TotalOutputTokens > 0) {
		totalIn := m.TotalInputTokens
		totalOut := m.TotalOutputTokens
		avgIn := float64(m.TotalInputTokens) / float64(m.Count)
		avgOut := float64(m.TotalOutputTokens) / float64(m.Count)
		minIn := m.MinInputTokens
		maxIn := m.MaxInputTokens
		minOut := m.MinOutputTokens
		maxOut := m.MaxOutputTokens

		// Reset sentinel values for display
		if minIn == math.MaxInt64 {
			minIn = 0
		}
		if minOut == math.MaxInt64 {
			minOut = 0
		}

		snap.TotalInputTokens = &totalIn
		snap.TotalOutputTokens = &totalOut
		snap.AvgInputTokens = &avgIn
		snap.AvgOutputTokens = &avgOut
		snap.MinInputTokens = &minIn
		snap.MaxInputTokens = &maxIn
		snap.MinOutputTokens = &minOut
		snap.MaxOutputTokens = &maxOut
	}

	return snap
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Snapshot{
		UptimeSeconds: time.Since(c.startTime).Seconds(),
		Embedding:     snapshotOp(c.ops[OpEmbedding], false),
		LLMGenerate:   snapshotOp(c.ops[OpLLMGenerate], true),
		LLMStream:     snapshotOp(c.ops[OpLLMStream], true),
		DBQuery:       snapshotOp(c.ops[OpDBQuery], false),
		DBSearch:      snapshotOp(c.ops[OpDBSearch], false),
	}
}
