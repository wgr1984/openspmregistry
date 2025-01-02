package utils

import (
	"container/list"
	"sync"
	"time"
)

type LRUCache[T any] struct {
	capacity  int
	ttl       time.Duration
	cache     map[string]*list.Element
	evictList *list.List
	mu        sync.Mutex
}

type entry[T any] struct {
	key       string
	value     T
	timestamp time.Time
}

func NewLRUCache[T any](capacity int, ttl time.Duration) *LRUCache[T] {
	return &LRUCache[T]{
		capacity:  capacity,
		ttl:       ttl,
		cache:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// Get returns the value (if any) and a boolean
// representing whether the value was found or not.
func (c *LRUCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.evictList.MoveToFront(ele)
		if time.Since(ele.Value.(*entry[T]).timestamp) < c.ttl {
			return ele.Value.(*entry[T]).value, true
		}
		c.removeElement(ele)
	}
	var zero T
	return zero, false
}

// Add adds a value to the cache.
// If the key already exists, it will update
// the value and move the element to the front.
func (c *LRUCache[T]) Add(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.evictList.MoveToFront(ele)
		ele.Value.(*entry[T]).value = value
		ele.Value.(*entry[T]).timestamp = time.Now()
		return
	}

	c.cache[key] = c.evictList.PushFront(&entry[T]{key, value, time.Now()})

	if c.evictList.Len() > c.capacity {
		c.removeOldest()
	}
}

// Remove removes a key from the cache.
// It returns the element that was removed.
// If the key does not exist, it returns nil.
func (c *LRUCache[T]) Remove(key string) *list.Element {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.removeElement(ele)
		return ele
	}
	return nil
}

// Remove removes a key from the cache.
func (c *LRUCache[T]) removeOldest() {
	ele := c.evictList.Back()
	if ele != nil {
		c.removeElement(ele)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRUCache[T]) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	delete(c.cache, e.Value.(*entry[T]).key)
}
