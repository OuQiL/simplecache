package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

type Cache struct {
	shards     []*shard
	shardMask  uint64
	config     Config
	closeChan  chan struct{}
	closeOnce  sync.Once
	entryCount int64
}

func New(config Config) (*Cache, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	shards := make([]*shard, config.ShardCount)
	for i := 0; i < config.ShardCount; i++ {
		shards[i] = newShard(config.MaxBytesPerShard, config.OnEvicted)
	}

	c := &Cache{
		shards:    shards,
		shardMask: uint64(config.ShardCount - 1),
		config:    config,
		closeChan: make(chan struct{}),
	}

	if config.CleanInterval > 0 {
		go c.cleanupLoop(config.CleanInterval)
	}

	return c, nil
}

func (c *Cache) getShard(hash uint64) *shard {
	return c.shards[hash&c.shardMask]
}

func (c *Cache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.config.DefaultTTL)
}

func (c *Cache) SetWithTTL(key string, value []byte, ttl time.Duration) error {
	hash := hashKey(key)
	shard := c.getShard(hash)

	var expireAt int64
	if ttl > 0 {
		expireAt = time.Now().Add(ttl).UnixNano()
	} else if c.config.DefaultTTL > 0 {
		expireAt = time.Now().Add(c.config.DefaultTTL).UnixNano()
	}

	delta, err := shard.set(key, hash, value, expireAt)
	if err != nil {
		return err
	}

	if delta != 0 {
		atomic.AddInt64(&c.entryCount, int64(delta))
	}

	return nil
}

func (c *Cache) Get(key string) ([]byte, bool) {
	hash := hashKey(key)
	shard := c.getShard(hash)
	return shard.get(key, hash)
}

func (c *Cache) Delete(key string) {
	hash := hashKey(key)
	shard := c.getShard(hash)
	delta := shard.delete(key, hash)
	if delta != 0 {
		atomic.AddInt64(&c.entryCount, int64(delta))
	}
}

func (c *Cache) Exists(key string) bool {
	_, found := c.Get(key)
	return found
}

func (c *Cache) Len() int {
	return int(atomic.LoadInt64(&c.entryCount))
}

func (c *Cache) Clear() {
	for _, shard := range c.shards {
		delta := shard.clear()
		if delta != 0 {
			atomic.AddInt64(&c.entryCount, int64(delta))
		}
	}
}

func (c *Cache) Close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		c.Clear()
	})
}

func (c *Cache) CloseChan() <-chan struct{} {
	return c.closeChan
}

func (c *Cache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.closeChan:
			return
		}
	}
}

func (c *Cache) cleanupExpired() {
	for _, shard := range c.shards {
		delta := shard.cleanupExpired()
		if delta != 0 {
			atomic.AddInt64(&c.entryCount, int64(delta))
		}
	}
}
