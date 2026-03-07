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
