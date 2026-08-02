package vault

import (
	"sync"
	"time"
)

type cacheEntry struct {
	value   string
	expires time.Time
}

// Cache stores fetched secret values with TTL-based expiration.
type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     int // seconds; 0 = no caching
}

// NewCache creates a cache with the given TTL in seconds.
// A TTL of 0 disables caching (Get always misses, Set is a no-op).
func NewCache(ttlSeconds int) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttlSeconds,
	}
}

// Get returns the cached value and true if the key exists and has not expired.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		return "", false
	}
	return e.value, true
}

// Set stores a value with the configured TTL. No-op when TTL is 0.
func (c *Cache) Set(key, value string) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		value:   value,
		expires: time.Now().Add(time.Duration(c.ttl) * time.Second),
	}
}

// Invalidate removes a specific key (for refresh-on-failure).
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// InvalidateAll removes all cached entries.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}
