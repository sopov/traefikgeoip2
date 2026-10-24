package traefikgeoip2

import (
	"container/list"
	"sync"
	"time"
)

const cacheShardCount = 16

// resultCache is a bounded, sharded LRU cache of lookup results with a TTL.
// Each shard holds at most perShard entries; the least recently used entry of a
// full shard is evicted on insert. Values are *GeoIPResult and are never
// modified after insertion.
type resultCache struct {
	ttl      time.Duration
	perShard int
	shards   [cacheShardCount]cacheShard
}

type cacheShard struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List // front = most recently used
}

type cacheEntry struct {
	key     string
	rec     *GeoIPResult
	expires time.Time
}

// newResultCache returns nil (caching disabled) when size is not positive.
func newResultCache(size int, ttl time.Duration) *resultCache {
	if size <= 0 {
		return nil
	}
	perShard := (size + cacheShardCount - 1) / cacheShardCount
	c := &resultCache{ttl: ttl, perShard: perShard}
	for i := range c.shards {
		c.shards[i].items = make(map[string]*list.Element, perShard)
		c.shards[i].order = list.New()
	}
	return c
}

func (c *resultCache) shard(key string) *cacheShard {
	// FNV-1a; small, allocation free and interpreter friendly.
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &c.shards[h%cacheShardCount]
}

func (c *resultCache) get(key string, now time.Time) (*GeoIPResult, bool) {
	s := c.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*cacheEntry) //nolint:errcheck // only cacheEntry values are stored
	if now.After(entry.expires) {
		s.order.Remove(el)
		delete(s.items, key)
		return nil, false
	}
	s.order.MoveToFront(el)
	return entry.rec, true
}

func (c *resultCache) set(key string, rec *GeoIPResult, now time.Time) {
	s := c.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.items[key]; ok {
		entry := el.Value.(*cacheEntry) //nolint:errcheck // only cacheEntry values are stored
		entry.rec = rec
		entry.expires = now.Add(c.ttl)
		s.order.MoveToFront(el)
		return
	}
	for s.order.Len() >= c.perShard {
		oldest := s.order.Back()
		s.order.Remove(oldest)
		delete(s.items, oldest.Value.(*cacheEntry).key) //nolint:errcheck // only cacheEntry values are stored
	}
	s.items[key] = s.order.PushFront(&cacheEntry{key: key, rec: rec, expires: now.Add(c.ttl)})
}

// len returns the number of entries across all shards, expired ones included.
func (c *resultCache) len() int {
	n := 0
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		n += s.order.Len()
		s.mu.Unlock()
	}
	return n
}

// purge drops every entry; used to release a superseded database's cache.
func (c *resultCache) purge() {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		s.items = make(map[string]*list.Element)
		s.order.Init()
		s.mu.Unlock()
	}
}
