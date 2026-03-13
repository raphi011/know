package agent

import (
	"sync"
	"testing"
)

func TestRingBuffer_PushAndSubscribe(t *testing.T) {
	rb := NewRingBuffer[int](5)

	// Push some items
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	// Subscribe gets history + live channel
	history, ch, unsub := rb.Subscribe()
	defer unsub()

	if len(history) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(history))
	}
	if history[0] != 1 || history[1] != 2 || history[2] != 3 {
		t.Fatalf("unexpected history: %v", history)
	}

	// Push after subscribe delivers to channel
	rb.Push(4)
	got := <-ch
	if got != 4 {
		t.Fatalf("expected 4 from channel, got %d", got)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4) // overwrites 1
	rb.Push(5) // overwrites 2

	history, _, unsub := rb.Subscribe()
	defer unsub()

	if len(history) != 3 {
		t.Fatalf("expected 3 items, got %d", len(history))
	}
	if history[0] != 3 || history[1] != 4 || history[2] != 5 {
		t.Fatalf("expected [3,4,5], got %v", history)
	}
}

func TestRingBuffer_Close(t *testing.T) {
	rb := NewRingBuffer[int](10)
	rb.Push(1)

	_, ch, _ := rb.Subscribe()

	rb.Close()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after Close()")
	}

	// Push after close is a no-op
	rb.Push(2)
	history, ch2, _ := rb.Subscribe()
	if len(history) != 1 {
		t.Fatalf("expected 1 item after close, got %d", len(history))
	}

	// New subscriber on closed buffer gets closed channel
	_, ok = <-ch2
	if ok {
		t.Fatal("expected new subscriber channel to be closed")
	}
}

func TestRingBuffer_Unsub(t *testing.T) {
	rb := NewRingBuffer[int](10)

	_, ch, unsub := rb.Subscribe()
	unsub()

	// Channel should be closed after unsub
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsub")
	}

	// Double unsub should not panic
	unsub()
}

func TestRingBuffer_MultipleSubscribers(t *testing.T) {
	rb := NewRingBuffer[int](10)

	_, ch1, unsub1 := rb.Subscribe()
	defer unsub1()
	_, ch2, unsub2 := rb.Subscribe()
	defer unsub2()

	rb.Push(42)

	got1 := <-ch1
	got2 := <-ch2
	if got1 != 42 || got2 != 42 {
		t.Fatalf("expected both subscribers to get 42, got %d and %d", got1, got2)
	}
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer[int](100)
	var wg sync.WaitGroup

	// Concurrent pushes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				rb.Push(v*10 + j)
			}
		}(i)
	}

	// Concurrent subscribe/unsub
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, unsub := rb.Subscribe()
			unsub()
		}()
	}

	wg.Wait()
	rb.Close()
}
