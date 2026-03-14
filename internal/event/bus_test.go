package event

import (
	"sync"
	"testing"
	"time"
)

func TestBus_PublishSubscribe(t *testing.T) {
	bus := New()
	ch, unsub := bus.Subscribe("vault:default")
	defer unsub()

	want := ChangeEvent{
		Type:    "document.created",
		VaultID: "vault:default",
		Payload: DocumentPayload{
			DocID:       "doc:1",
			Path:        "notes/hello.md",
			ContentHash: "abc123",
		},
	}

	bus.Publish(want)

	select {
	case got := <-ch:
		if got.Type != want.Type {
			t.Errorf("Type = %q, want %q", got.Type, want.Type)
		}
		if got.VaultID != want.VaultID {
			t.Errorf("VaultID = %q, want %q", got.VaultID, want.VaultID)
		}
		p, ok := got.Payload.(DocumentPayload)
		if !ok {
			t.Fatalf("Payload type = %T, want DocumentPayload", got.Payload)
		}
		if p.DocID != "doc:1" {
			t.Errorf("Payload.DocID = %q, want %q", p.DocID, "doc:1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := New()

	ch1, unsub1 := bus.Subscribe("vault:a")
	defer unsub1()

	ch2, unsub2 := bus.Subscribe("vault:a")
	defer unsub2()

	event := ChangeEvent{
		Type:    "document.updated",
		VaultID: "vault:a",
		Payload: DocumentPayload{DocID: "doc:2", Path: "readme.md", ContentHash: "def"},
	}

	bus.Publish(event)

	for i, ch := range []<-chan ChangeEvent{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Type != event.Type {
				t.Errorf("subscriber %d: Type = %q, want %q", i, got.Type, event.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestBus_VaultIsolation(t *testing.T) {
	bus := New()

	chA, unsubA := bus.Subscribe("vault:a")
	defer unsubA()

	chB, unsubB := bus.Subscribe("vault:b")
	defer unsubB()

	bus.Publish(ChangeEvent{
		Type:    "document.created",
		VaultID: "vault:a",
		Payload: DocumentPayload{DocID: "doc:1", Path: "a.md", ContentHash: "x"},
	})

	// Subscriber A should receive the event.
	select {
	case <-chA:
	case <-time.After(time.Second):
		t.Fatal("vault:a subscriber did not receive event")
	}

	// Subscriber B should NOT receive the event.
	select {
	case ev := <-chB:
		t.Fatalf("vault:b subscriber received unexpected event: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event for vault:b.
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := New()

	ch, unsub := bus.Subscribe("vault:default")

	// Unsubscribe should close the channel.
	unsub()

	// Channel should be closed (reads return zero value, ok=false).
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe should not panic.
	bus.Publish(ChangeEvent{
		Type:    "document.deleted",
		VaultID: "vault:default",
		Payload: DocumentPayload{DocID: "doc:1", Path: "gone.md", ContentHash: "z"},
	})

	// Calling unsubscribe again should be safe.
	unsub()
}

func TestBus_SlowConsumer(t *testing.T) {
	bus := New()

	ch, unsub := bus.Subscribe("vault:default")
	defer unsub()

	// Fill the channel buffer (capacity 64).
	for i := range 64 {
		bus.Publish(ChangeEvent{
			Type:    "document.updated",
			VaultID: "vault:default",
			Payload: DocumentPayload{DocID: "doc:fill", Path: "fill.md", ContentHash: string(rune('a' + i%26))},
		})
	}

	// Next publish should evict the slow consumer and close the channel.
	bus.Publish(ChangeEvent{
		Type:    "document.updated",
		VaultID: "vault:default",
		Payload: DocumentPayload{DocID: "doc:overflow", Path: "overflow.md", ContentHash: "over"},
	})

	// Drain the buffered events first.
	drained := 0
	for range ch {
		drained++
	}

	// Channel should be closed; we should have drained exactly 64 events.
	if drained != 64 {
		t.Errorf("drained %d events, want 64", drained)
	}

	// Verify the channel is closed.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after slow consumer eviction")
	}
}

func TestBus_SubscribeByPath(t *testing.T) {
	bus := New()

	ch, unsub := bus.SubscribeByPath("vault:default", "/docs/readme.md")
	defer unsub()

	// Publish event for matching path
	bus.Publish(ChangeEvent{
		Type:    "document.updated",
		VaultID: "vault:default",
		Payload: DocumentPayload{DocID: "doc:1", Path: "/docs/readme.md", ContentHash: "abc"},
	})

	// Publish event for non-matching path
	bus.Publish(ChangeEvent{
		Type:    "document.updated",
		VaultID: "vault:default",
		Payload: DocumentPayload{DocID: "doc:2", Path: "/docs/other.md", ContentHash: "def"},
	})

	// Should receive only the matching event
	select {
	case got := <-ch:
		p := got.Payload.(DocumentPayload)
		if p.Path != "/docs/readme.md" {
			t.Errorf("Path = %q, want %q", p.Path, "/docs/readme.md")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for matching event")
	}

	// Should not receive the non-matching event
	select {
	case got := <-ch:
		t.Fatalf("received unexpected event: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// Expected: no more events
	}
}

func TestBus_SubscribeByPath_OldPath(t *testing.T) {
	bus := New()

	ch, unsub := bus.SubscribeByPath("vault:default", "/docs/old.md")
	defer unsub()

	// Publish move event where OldPath matches
	bus.Publish(ChangeEvent{
		Type:    "document.moved",
		VaultID: "vault:default",
		Payload: DocumentPayload{DocID: "doc:1", Path: "/docs/new.md", OldPath: "/docs/old.md", ContentHash: "abc"},
	})

	select {
	case got := <-ch:
		p := got.Payload.(DocumentPayload)
		if p.OldPath != "/docs/old.md" {
			t.Errorf("OldPath = %q, want %q", p.OldPath, "/docs/old.md")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for move event")
	}
}

func TestBus_ConcurrentPublish(t *testing.T) {
	bus := New()

	const numSubscribers = 5
	const numPublishers = 10
	const eventsPerPublisher = 100

	type result struct {
		events []ChangeEvent
	}

	results := make([]result, numSubscribers)
	var wg sync.WaitGroup

	// Start subscribers.
	unsubs := make([]func(), numSubscribers)
	for i := range numSubscribers {
		ch, unsub := bus.Subscribe("vault:concurrent")
		unsubs[i] = unsub

		wg.Add(1)
		go func(idx int, ch <-chan ChangeEvent) {
			defer wg.Done()
			for ev := range ch {
				results[idx].events = append(results[idx].events, ev)
			}
		}(i, ch)
	}

	// Start publishers concurrently.
	var pubWg sync.WaitGroup
	for i := range numPublishers {
		pubWg.Add(1)
		go func(publisherID int) {
			defer pubWg.Done()
			for j := range eventsPerPublisher {
				bus.Publish(ChangeEvent{
					Type:    "document.updated",
					VaultID: "vault:concurrent",
					Payload: DocumentPayload{
						DocID:       "doc:concurrent",
						Path:        "concurrent.md",
						ContentHash: string(rune(publisherID*1000 + j)),
					},
				})
			}
		}(i)
	}

	// Wait for all publishers to finish, then unsubscribe to close channels.
	pubWg.Wait()
	for _, unsub := range unsubs {
		unsub()
	}

	// Wait for all subscriber goroutines to drain.
	wg.Wait()

	totalExpected := numPublishers * eventsPerPublisher
	for i, r := range results {
		// Subscribers might have been evicted if the channel filled up,
		// so we just check they received at least some events and no more
		// than the total published.
		if len(r.events) == 0 {
			t.Errorf("subscriber %d received 0 events", i)
		}
		if len(r.events) > totalExpected {
			t.Errorf("subscriber %d received %d events, max expected %d", i, len(r.events), totalExpected)
		}
	}
}

func TestBus_Close(t *testing.T) {
	bus := New()

	ch1, _ := bus.Subscribe("vault:a")
	ch2, _ := bus.Subscribe("vault:b")

	bus.Close()

	// All subscriber channels should be closed.
	if _, ok := <-ch1; ok {
		t.Error("ch1 should be closed after Close")
	}
	if _, ok := <-ch2; ok {
		t.Error("ch2 should be closed after Close")
	}

	// Publish after Close should not panic.
	bus.Publish(ChangeEvent{
		Type:    "document.created",
		VaultID: "vault:a",
		Payload: DocumentPayload{DocID: "doc:1", Path: "a.md", ContentHash: "x"},
	})

	// Subscribe after Close returns a closed channel.
	ch3, unsub3 := bus.Subscribe("vault:c")
	defer unsub3()
	if _, ok := <-ch3; ok {
		t.Error("ch3 should be closed when subscribing to a closed bus")
	}

	// Close is idempotent.
	bus.Close()
}

func TestBus_Close_WithUnsubscribe(t *testing.T) {
	bus := New()

	_, unsub := bus.Subscribe("vault:a")

	bus.Close()

	// Calling unsubscribe after Close should not panic or deadlock.
	unsub()
}

func TestBus_Close_ConcurrentPublish(t *testing.T) {
	bus := New()

	const numPublishers = 10

	ch, unsub := bus.Subscribe("vault:a")
	defer unsub()

	var wg sync.WaitGroup
	for range numPublishers {
		wg.Go(func() {
			for i := range 100 {
				bus.Publish(ChangeEvent{
					Type:    "document.updated",
					VaultID: "vault:a",
					Payload: DocumentPayload{DocID: "doc:1", Path: "a.md", ContentHash: string(rune(i))},
				})
			}
		})
	}

	// Close while publishers are active — must not panic.
	bus.Close()
	wg.Wait()

	// Channel should be closed.
	for range ch {
		// drain
	}
}

func TestBus_Close_ConcurrentSubscribe(t *testing.T) {
	bus := New()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				ch, unsub := bus.Subscribe("vault:a")
				// Read one event or notice the channel is closed.
				select {
				case <-ch:
				default:
				}
				unsub()
			}
		})
	}

	// Close while subscribers are registering — must not panic.
	bus.Close()
	wg.Wait()
}

func TestBus_Close_SubscribeByPath(t *testing.T) {
	bus := New()

	ch, unsub := bus.SubscribeByPath("vault:a", "/docs/readme.md")
	defer unsub()

	bus.Close()

	// The filtered channel should close once the underlying vault channel closes.
	for range ch {
		// drain
	}

	// Channel is closed.
	_, ok := <-ch
	if ok {
		t.Error("filtered channel should be closed after bus Close")
	}
}
