package webdav

import (
	"sync"
	"testing"
	"time"
)

func TestPendingSet_AddHasRemove(t *testing.T) {
	ps := newPendingSet()

	if ps.Has("/test.md") {
		t.Error("expected Has to return false for unknown path")
	}

	ps.Add("/test.md")
	if !ps.Has("/test.md") {
		t.Error("expected Has to return true after Add")
	}

	if !ps.Remove("/test.md") {
		t.Error("expected Remove to return true for existing path")
	}
	if ps.Has("/test.md") {
		t.Error("expected Has to return false after Remove")
	}
	if ps.Remove("/test.md") {
		t.Error("expected Remove to return false for already-removed path")
	}
}

func TestPendingSet_Sweep(t *testing.T) {
	ps := newPendingSet()

	// Add an entry with a backdated claimedAt
	ps.mu.Lock()
	ps.paths["/old.md"] = time.Now().Add(-2 * time.Minute)
	ps.paths["/new.md"] = time.Now()
	ps.mu.Unlock()

	ps.Sweep(1 * time.Minute)

	if ps.Has("/old.md") {
		t.Error("expected old entry to be swept")
	}
	if !ps.Has("/new.md") {
		t.Error("expected new entry to survive sweep")
	}
}

func TestPendingSet_ConcurrentAccess(t *testing.T) {
	ps := newPendingSet()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(3)
		path := "/file" + string(rune('a'+i%26)) + ".md"
		go func() {
			defer wg.Done()
			ps.Add(path)
		}()
		go func() {
			defer wg.Done()
			ps.Has(path)
		}()
		go func() {
			defer wg.Done()
			ps.Remove(path)
		}()
	}

	wg.Wait()
	// No panic = success for concurrent access test
}
