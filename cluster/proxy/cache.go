package proxy

import (
	"errors"
	"sync"
	"time"
)

type TimeSource interface {
	Now() time.Time
}

type StdTimeSource struct{}

func (s *StdTimeSource) Now() time.Time {
	return time.Now()
}

type CacheEntry[T any] struct {
	value      T
	validUntil time.Time
}

type Cache[T any] struct {
	ts        TimeSource
	lock      sync.Mutex
	entries   map[string]CacheEntry[T]
	lastPurge time.Time
}

func NewCache[T any](ts TimeSource) *Cache[T] {
	c := &Cache[T]{
		ts:      ts,
		entries: map[string]CacheEntry[T]{},
	}

	if c.ts == nil {
		c.ts = &StdTimeSource{}
	}

	c.lastPurge = c.ts.Now()

	return c
}

func (c *Cache[T]) Get(key string) (T, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	e, ok := c.entries[key]
	if !ok {
		var noop T
		return noop, errors.New("not found")
	}

	if c.ts.Now().After(e.validUntil) {
		delete(c.entries, key)
		var noop T
		return noop, errors.New("not found")
	}

	return e.value, nil
}

func (c *Cache[T]) Put(key string, value T, ttl time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()

	now := c.ts.Now()

	if now.Sub(c.lastPurge) > time.Minute {
		c.purge(now)
		c.lastPurge = now
	}

	e := c.entries[key]

	e.value = value
	e.validUntil = now.Add(ttl)

	c.entries[key] = e
}

func (c *Cache[T]) Delete(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.entries, key)
}

func (c *Cache[T]) purge(now time.Time) {
	for key, e := range c.entries {
		if now.After(e.validUntil) {
			delete(c.entries, key)
		}
	}
}
