package cache

import "time"

type Config struct {
	ShardCount       int
	DefaultTTL       time.Duration
	CleanInterval    time.Duration
	OnEvicted        func(key string, value []byte)
	MaxBytesPerShard int
}

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
