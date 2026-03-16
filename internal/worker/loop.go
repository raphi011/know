// Package worker provides shared background worker infrastructure.
package worker

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
)

// WorkerLoop encapsulates the common run/restart/tick pattern for background workers.
type WorkerLoop struct {
	name         string
	interval     time.Duration
	restartDelay time.Duration // delay after a panic before restarting (default 5s)
	tick         func(ctx context.Context)
	notify       <-chan struct{} // optional event-driven wake-up (nil = poll only)
}

// NewWorkerLoop creates a worker loop that calls tick on each interval.
// If notify is non-nil, the loop also wakes on signals from that channel.
func NewWorkerLoop(name string, interval time.Duration, tick func(ctx context.Context), notify <-chan struct{}) *WorkerLoop {
	return &WorkerLoop{
		name:         name,
		interval:     interval,
		restartDelay: 5 * time.Second,
		tick:         tick,
		notify:       notify,
	}
}

// WithRestartDelay sets the delay after a panic before the loop restarts.
// Useful in tests to avoid waiting 5 seconds.
func (w *WorkerLoop) WithRestartDelay(d time.Duration) *WorkerLoop {
	w.restartDelay = d
	return w
}

// Run starts the worker loop. It blocks until ctx is cancelled.
// If the loop panics, it logs the stack trace and restarts after a short delay.
func (w *WorkerLoop) Run(ctx context.Context) {
	logutil.FromCtx(ctx).Info(w.name+" started", "interval", w.interval)

	for {
		stopped := w.runLoop(ctx)
		if stopped {
			return
		}
		select {
		case <-ctx.Done():
			logutil.FromCtx(ctx).Info(w.name + " stopped")
			return
		case <-time.After(w.restartDelay):
			logutil.FromCtx(ctx).Info(w.name + " restarting after panic")
		}
	}
}

func (w *WorkerLoop) runLoop(ctx context.Context) (stopped bool) {
	defer func() {
		if p := recover(); p != nil {
			logutil.FromCtx(ctx).Error(w.name+" panicked",
				"error", p, "stack", string(debug.Stack()))
			stopped = false
		}
	}()

	// Process any pending work immediately on startup
	w.tick(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logutil.FromCtx(ctx).Info(w.name + " stopped")
			return true
		case <-ticker.C:
			w.tick(ctx)
		case <-w.notify:
			w.tick(ctx)
		}
	}
}

// EventNotify subscribes globally to the bus and returns a coalescing notification
// channel plus an unsubscribe function. The channel receives a signal whenever an
// event matching one of the given types is published on any vault.
// If bus is nil, returns (nil, no-op) — the worker falls back to polling only.
// The caller must eventually call the returned unsubscribe function (or close the
// bus) to stop the internal goroutine.
//
// Must be called from Run (not from a constructor) to avoid goroutine leaks if
// the worker is constructed but never started.
func EventNotify(bus *event.Bus, eventTypes ...string) (<-chan struct{}, func()) {
	if bus == nil {
		return nil, func() {}
	}

	typeSet := make(map[string]struct{}, len(eventTypes))
	for _, t := range eventTypes {
		typeSet[t] = struct{}{}
	}

	events, unsub := bus.SubscribeGlobal()
	notify := make(chan struct{}, 1)

	go func() {
		for evt := range events {
			if _, ok := typeSet[evt.Type]; ok {
				select {
				case notify <- struct{}{}:
				default: // already signaled, coalesce
				}
			}
		}
	}()

	return notify, unsub
}
