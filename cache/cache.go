package cache

import "sync"

type Cacheable[K comparable] interface {
	CacheKey() K
}

// Cache is a goroutine safe write-through cache with generics type.
type Cache[K comparable, V Cacheable[K]] struct {
	mu    sync.RWMutex
	data  map[K]V
	read  func(K) (V, error)
	write func(V) error
}

// NewCache creates a new Cache.
func NewCache[K comparable, V Cacheable[K]](read func(K) (V, error), write func(V) error) *Cache[K, V] {
	return &Cache[K, V]{
		mu:    sync.RWMutex{},
		data:  make(map[K]V),
		read:  read,
		write: write,
	}
}

// Get returns a value from the cache.
func (c *Cache[K, V]) Get(key K) (*V, error) {
	c.mu.RLock()
	v, ok := c.data[key]
	c.mu.RUnlock()
	if ok {
		return &v, nil // cache hit
	}

	v, err := c.read(key)
	if err != nil {
		return nil, err // read error
	}

	c.mu.Lock()
	c.data[key] = v
	c.mu.Unlock()

	return &v, nil
}

// Set sets a value to the cache.
func (c *Cache[K, V]) Set(key K, value V) error {
	err := c.write(value)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.data[key] = value
	c.mu.Unlock()

	return nil
}
