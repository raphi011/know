package webdav

import (
	"log/slog"
	"sync"
	"time"
)

// pendingSet tracks paths that have been claimed but not yet persisted to the DB.
// This prevents ghost empty documents when Finder's two-phase PUT is interrupted.
// Each path maps to the time it was claimed, used by Sweep to expire stale entries.
type pendingSet struct {
	mu    sync.RWMutex
	paths map[string]time.Time
}

func newPendingSet() *pendingSet {
	return &pendingSet{paths: make(map[string]time.Time)}
}

// Add registers a path as pending (claimed but not yet written).
func (ps *pendingSet) Add(path string) {
	if ps == nil {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.paths[path] = time.Now()
}

// Remove removes a path from the pending set (file got real content or was deleted).
// Returns true if the path was present.
func (ps *pendingSet) Remove(path string) bool {
	if ps == nil {
		return false
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	_, ok := ps.paths[path]
	delete(ps.paths, path)
	return ok
}

// Has returns true if the path is in the pending set.
func (ps *pendingSet) Has(path string) bool {
	if ps == nil {
		return false
	}
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	_, ok := ps.paths[path]
	return ok
}

// Sweep removes entries older than ttl.
func (ps *pendingSet) Sweep(ttl time.Duration) {
	if ps == nil {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	for p, claimedAt := range ps.paths {
		if claimedAt.Before(cutoff) {
			slog.Debug("webdav: sweeping expired pending entry", "path", p, "claimedAt", claimedAt)
			delete(ps.paths, p)
		}
	}
}
