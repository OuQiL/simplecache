package simplecache

import "time"

type Config struct {
	ShardCount       int                            // 分片数量，必须是2的幂次方
	DefaultTTL       time.Duration                  // 默认过期时间，必须大于0
	CleanInterval    time.Duration                  // 清理过期条目的时间间隔
	OnEvicted        func(key string, value []byte) // 条目被驱逐时的回调函数
	MaxBytesPerShard int                            // 每个分片的环形缓冲区大小（字节）
}

/*
DefaultConfig 返回默认配置
  - 分片数量：256
  - 默认过期时间：ttl(等于0则为永不过期-假)
  - 清理过期条目的时间间隔：1分钟
  - 条目被驱逐时的回调函数：nil
  - 每个分片的环形缓冲区大小：4MB
*/
func DefaultConfig(ttl time.Duration) Config {
	return Config{
		ShardCount:       256,
		DefaultTTL:       ttl,
		CleanInterval:    time.Minute,
		OnEvicted:        nil,
		MaxBytesPerShard: 4 * 1024 * 1024,
	}
}

func (c Config) Validate() error {
	if c.ShardCount <= 0 || (c.ShardCount&(c.ShardCount-1)) != 0 {
		return ErrInvalidShardCount
	}

	if c.DefaultTTL < 0 {
		return ErrInvalidTTL
	}

	if c.MaxBytesPerShard <= 0 {
		return ErrInvalidMaxBytesPerShard
	}

	return nil
}
