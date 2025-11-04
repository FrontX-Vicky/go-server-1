package schema

import (
	"sync"
	"time"
)

// cache provides a short-lived in-memory cache for schema payloads.
type cache struct {
	ttl time.Duration

	mu        sync.RWMutex
	payload   *Payload
	expiresAt time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl}
}

func (c *cache) get() (*Payload, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.payload == nil || time.Now().After(c.expiresAt) {
		return nil, false
	}
	return c.payload, true
}

func (c *cache) set(p *Payload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p == nil {
		c.payload = nil
		c.expiresAt = time.Time{}
		return
	}
	c.payload = p
	c.expiresAt = time.Now().Add(c.ttl)
}

func (c *cache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.payload = nil
	c.expiresAt = time.Time{}
}
