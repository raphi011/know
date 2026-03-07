package event

import (
	"log/slog"
	"sync"
)

// ChangeEvent represents a change notification scoped to a vault.
type ChangeEvent struct {
	Type    string `json:"type"`    // "document.created", "document.updated", etc.
	VaultID string `json:"vaultId"`
	Payload any    `json:"payload"`
}

// DocumentPayload carries document-specific change details.
type DocumentPayload struct {
	DocID       string `json:"docId"`
	Path        string `json:"path"`
	OldPath     string `json:"oldPath,omitempty"`
	ContentHash string `json:"contentHash"`
}

// Bus is an in-process pub/sub event bus that fans out change events
// to subscribers grouped by vault ID.
type Bus struct {
	mu   sync.Mutex
	subs map[string]map[uint64]chan ChangeEvent // vaultID → subID → channel
	next uint64
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subs: make(map[string]map[uint64]chan ChangeEvent),
	}
}

// Subscribe registers a new subscriber for the given vault ID.
// It returns a receive-only channel and an unsubscribe function.
// The channel is buffered with capacity 64; slow consumers that
// let the buffer fill will have their channel closed and subscription removed.
// The unsubscribe function is safe to call multiple times.
func (b *Bus) Subscribe(vaultID string) (ch <-chan ChangeEvent, unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.next
	b.next++

	c := make(chan ChangeEvent, 64)

	if b.subs[vaultID] == nil {
		b.subs[vaultID] = make(map[uint64]chan ChangeEvent)
	}
	b.subs[vaultID][id] = c

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			if _, ok := b.subs[vaultID][id]; ok {
				close(c)
				delete(b.subs[vaultID], id)
				if len(b.subs[vaultID]) == 0 {
					delete(b.subs, vaultID)
				}
			}
		})
	}

	return c, unsubscribe
}

// SubscribeByPath registers a subscriber that only receives events matching a
// specific document path within a vault. It wraps Subscribe with client-side
// filtering. The returned channel receives only events where the DocumentPayload
// path matches the given path.
func (b *Bus) SubscribeByPath(vaultID, docPath string) (ch <-chan ChangeEvent, unsubscribe func()) {
	vaultCh, vaultUnsub := b.Subscribe(vaultID)

	filtered := make(chan ChangeEvent, 16)
	done := make(chan struct{})

	go func() {
		defer close(filtered)
		for {
			select {
			case evt, ok := <-vaultCh:
				if !ok {
					return
				}
				if p, ok := evt.Payload.(DocumentPayload); ok {
					if p.Path == docPath || p.OldPath == docPath {
						select {
						case filtered <- evt:
						default:
							slog.Debug("dropping filtered event for slow consumer",
							"vault", vaultID, "path", docPath, "type", evt.Type)
						}
					}
				}
			case <-done:
				return
			}
		}
	}()

	var once sync.Once
	unsubscribe = func() {
		once.Do(func() {
			close(done)
			vaultUnsub()
		})
	}

	return filtered, unsubscribe
}

// Publish fans out the event to all subscribers registered for the event's VaultID.
// If a subscriber's channel buffer is full, the channel is closed and the subscriber
// is evicted (slow consumer eviction).
func (b *Bus) Publish(event ChangeEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vaultSubs, ok := b.subs[event.VaultID]
	if !ok {
		return
	}

	for id, ch := range vaultSubs {
		select {
		case ch <- event:
		default:
			// Slow consumer: close channel and remove subscription.
			slog.Warn("evicting slow consumer",
				"vaultID", event.VaultID,
				"subID", id,
			)
			close(ch)
			delete(vaultSubs, id)
		}
	}

	if len(vaultSubs) == 0 {
		delete(b.subs, event.VaultID)
	}
}
