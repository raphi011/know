package agent

import "sync"

// RingBuffer is a thread-safe bounded buffer with pub/sub for event replay on reconnect.
// When the buffer is full, the oldest item is overwritten.
type RingBuffer[T any] struct {
	mu       sync.Mutex
	items    []T
	capacity int
	subs     map[uint64]chan T
	nextSub  uint64
	closed   bool
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		items:    make([]T, 0, capacity),
		capacity: capacity,
		subs:     make(map[uint64]chan T),
	}
}

// Push appends an item and broadcasts it to all subscribers.
// If a subscriber's channel is full, the event is dropped for that subscriber
// (they can reconnect and replay from the buffer).
func (rb *RingBuffer[T]) Push(item T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return
	}

	if len(rb.items) < rb.capacity {
		rb.items = append(rb.items, item)
	} else {
		// Shift items left and append at the end
		copy(rb.items, rb.items[1:])
		rb.items[rb.capacity-1] = item
	}

	for _, ch := range rb.subs {
		select {
		case ch <- item:
		default:
			// subscriber channel full, drop event
		}
	}
}

// Subscribe returns a snapshot of the current buffer contents plus a live channel
// for new items. Call unsub to stop receiving and clean up.
func (rb *RingBuffer[T]) Subscribe() (history []T, ch <-chan T, unsub func()) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Copy current items as history
	history = make([]T, len(rb.items))
	copy(history, rb.items)

	if rb.closed {
		// Buffer already closed — return history and a closed channel
		closed := make(chan T)
		close(closed)
		return history, closed, func() {}
	}

	live := make(chan T, 64)
	id := rb.nextSub
	rb.nextSub++
	rb.subs[id] = live

	unsub = func() {
		rb.mu.Lock()
		defer rb.mu.Unlock()
		if _, ok := rb.subs[id]; ok {
			delete(rb.subs, id)
			close(live)
		}
	}

	return history, live, unsub
}

// Close closes all subscriber channels. After Close, Push is a no-op.
func (rb *RingBuffer[T]) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return
	}
	rb.closed = true

	for id, ch := range rb.subs {
		close(ch)
		delete(rb.subs, id)
	}
}
