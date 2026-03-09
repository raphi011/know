package mcptools

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_HitAndMiss(t *testing.T) {
	c := newCache(100 * time.Millisecond)

	callCount := 0
	fetch := func() (string, error) {
		callCount++
		return "result", nil
	}

	// Miss
	v, err := c.GetOrFetch("key", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "result" {
		t.Fatalf("expected 'result', got %q", v)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	// Hit
	v, err = c.GetOrFetch("key", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "result" {
		t.Fatalf("expected 'result', got %q", v)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (cached), got %d", callCount)
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := newCache(10 * time.Millisecond)

	callCount := 0
	fetch := func() (string, error) {
		callCount++
		return fmt.Sprintf("v%d", callCount), nil
	}

	v1, _ := c.GetOrFetch("key", fetch)
	if v1 != "v1" {
		t.Fatalf("expected 'v1', got %q", v1)
	}

	time.Sleep(20 * time.Millisecond)

	v2, _ := c.GetOrFetch("key", fetch)
	if v2 != "v2" {
		t.Fatalf("expected 'v2' after expiry, got %q", v2)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls after expiry, got %d", callCount)
	}
}

func TestCache_FetchError(t *testing.T) {
	c := newCache(time.Minute)

	_, err := c.GetOrFetch("key", func() (string, error) {
		return "", fmt.Errorf("fetch failed")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// Should retry on next call since error wasn't cached
	v, err := c.GetOrFetch("key", func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "ok" {
		t.Fatalf("expected 'ok', got %q", v)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := newCache(time.Minute)

	var calls atomic.Int32
	fetch := func() (string, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond) // simulate work
		return "value", nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrFetch("key", fetch)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != "value" {
				t.Errorf("expected 'value', got %q", v)
			}
		}()
	}
	wg.Wait()

	// Singleflight should deduplicate: only 1 fetch call
	if n := calls.Load(); n != 1 {
		t.Errorf("expected 1 fetch call (singleflight), got %d", n)
	}
}

func TestCache_DifferentKeys(t *testing.T) {
	c := newCache(time.Minute)

	v1, _ := c.GetOrFetch("a", func() (string, error) { return "alpha", nil })
	v2, _ := c.GetOrFetch("b", func() (string, error) { return "beta", nil })

	if v1 != "alpha" {
		t.Fatalf("expected 'alpha', got %q", v1)
	}
	if v2 != "beta" {
		t.Fatalf("expected 'beta', got %q", v2)
	}
}
