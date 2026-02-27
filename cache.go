package simplecache

import (
	"sync"
	"sync/atomic"
	"time"
)

/*
Cache (缓存管理器)
├── shards []*shard  (256个分片，默认配置)
├── shardMask       (用于快速计算分片索引)
└── entryCount      (总条目数，原子操作)
*/
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

/*
getShard (获取分片)
├── hash (哈希值)
└── 返回值 (*shard)
*/
func (c *Cache) getShard(hash uint64) *shard {
	return c.shards[hash&c.shardMask]
}

func (c *Cache) Set(key string, value []byte) error {
	return c.SetWithTTL(key, value, c.config.DefaultTTL)
}

/*
SetWithTTL (设置键值对，带过期时间)
├── 计算哈希值 (根据键计算分片索引)
├── 获取分片 (根据哈希值获取对应的分片)
├── 计算过期时间 (根据TTL参数计算过期时间戳)
├── 调用分片的set方法 (将键值对写入分片)
├── 更新条目数 (如果有新条目被写入，更新缓存中的条目数)
*/
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

/*
Get (获取缓存值)
├── 调用分片的get方法
└── 返回值和是否存在
*/
func (c *Cache) Get(key string) ([]byte, bool) {
	hash := hashKey(key)
	shard := c.getShard(hash)
	return shard.get(key, hash)
}

/*
Delete (删除缓存键值对)
├── 计算哈希值 (根据键计算分片索引)
├── 获取分片 (根据哈希值获取对应的分片)
├── 调用分片的delete方法 (删除键值对)
├── 更新条目数 (如果有键值对被删除，更新缓存中的条目数)
*/
func (c *Cache) Delete(key string) {
	hash := hashKey(key)
	shard := c.getShard(hash)
	delta := shard.delete(key, hash)
	if delta != 0 {
		atomic.AddInt64(&c.entryCount, int64(delta))
	}
}

/*
Exists (检查键是否存在)
├── 调用Get方法 (获取键对应的值和是否存在)
└── 返回是否存在
*/
func (c *Cache) Exists(key string) bool {
	_, found := c.Get(key)
	return found
}

/*
Len (获取缓存条目数)
└── 返回值 (int)
*/
func (c *Cache) Len() int {
	return int(atomic.LoadInt64(&c.entryCount))
}

/*
Clear (清空缓存)
├── 遍历所有分片 (调用分片的clear方法)
├── 更新条目数 (如果有键值对被删除，更新缓存中的条目数)
*/
func (c *Cache) Clear() {
	for _, shard := range c.shards {
		delta := shard.clear()
		if delta != 0 {
			atomic.AddInt64(&c.entryCount, int64(delta))
		}
	}
}

/*
Close (关闭缓存)
├── 关闭关闭通道 (防止重复关闭)
├── 清空缓存 (调用Clear方法)
*/
func (c *Cache) Close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		c.Clear()
	})
}

/*
CloseChan (关闭通道)
└── 返回值 (chan struct{})
*/
func (c *Cache) CloseChan() <-chan struct{} {
	return c.closeChan
}

// 清理过期键值对，后台执行
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
