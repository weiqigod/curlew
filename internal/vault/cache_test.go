package vault

import (
	"sync"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	t.Run("hit_within_ttl", func(t *testing.T) {
		c := NewCache(60)
		c.Set("key", "value")
		got, ok := c.Get("key")
		if !ok {
			t.Fatal("expected cache hit")
		}
		if got != "value" {
			t.Errorf("got %q, want %q", got, "value")
		}
	})

	t.Run("miss_on_empty_cache", func(t *testing.T) {
		c := NewCache(60)
		_, ok := c.Get("nonexistent")
		if ok {
			t.Fatal("expected cache miss on empty cache")
		}
	})

	t.Run("expired_after_ttl", func(t *testing.T) {
		c := NewCache(1) // 1 second TTL
		c.Set("key", "value")

		// Manually expire the entry
		c.mu.Lock()
		c.entries["key"] = cacheEntry{value: "value", expires: time.Now().Add(-1 * time.Second)}
		c.mu.Unlock()

		_, ok := c.Get("key")
		if ok {
			t.Fatal("expected cache miss after expiry")
		}
	})

	t.Run("zero_ttl_disables_caching", func(t *testing.T) {
		c := NewCache(0)
		c.Set("key", "value")
		_, ok := c.Get("key")
		if ok {
			t.Fatal("expected cache miss with zero TTL")
		}
	})

	t.Run("invalidate_single_key", func(t *testing.T) {
		c := NewCache(60)
		c.Set("a", "1")
		c.Set("b", "2")
		c.Invalidate("a")

		_, okA := c.Get("a")
		_, okB := c.Get("b")
		if okA {
			t.Fatal("expected cache miss for invalidated key 'a'")
		}
		if !okB {
			t.Fatal("expected cache hit for key 'b'")
		}
	})

	t.Run("invalidate_all_clears_everything", func(t *testing.T) {
		c := NewCache(60)
		c.Set("a", "1")
		c.Set("b", "2")
		c.InvalidateAll()

		_, okA := c.Get("a")
		_, okB := c.Get("b")
		if okA || okB {
			t.Fatal("expected all cache entries cleared")
		}
	})

	t.Run("concurrent_access_is_safe", func(t *testing.T) {
		c := NewCache(60)
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				key := "key"
				c.Set(key, "value")
				c.Get(key)
				c.Invalidate(key)
			}(i)
		}
		wg.Wait()
		// No race condition — test passes if no panic/race detector error
	})
}
