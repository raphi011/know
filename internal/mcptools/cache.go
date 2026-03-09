package mcptools

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

type cache struct {
	entries sync.Map
	group   singleflight.Group
	ttl     time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl}
}

// GetOrFetch returns a cached value if present and not expired,
// otherwise calls fetch, caches the result, and returns it.
// Errors are not cached, so subsequent calls will retry.
// Concurrent calls for the same key are deduplicated via singleflight.
func (c *cache) GetOrFetch(key string, fetch func() (string, error)) (string, error) {
	if v, ok := c.entries.Load(key); ok {
		entry := v.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.value, nil
		}
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		// Double-check after winning the singleflight race
		if v, ok := c.entries.Load(key); ok {
			entry := v.(cacheEntry)
			if time.Now().Before(entry.expiresAt) {
				return entry.value, nil
			}
		}

		value, err := fetch()
		if err != nil {
			return "", err
		}

		c.entries.Store(key, cacheEntry{
			value:     value,
			expiresAt: time.Now().Add(c.ttl),
		})
		return value, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
