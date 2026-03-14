package cache

import "errors"

var (
	ErrInvalidShardCount       = errors.New("shard count must be a power of 2 and greater than 0")
	ErrInvalidTTL              = errors.New("default TTL must be greater than 0 or equal to 0")
	ErrInvalidMaxBytesPerShard = errors.New("max bytes per shard must be greater than 0")
	ErrEmptyKey                = errors.New("key cannot be empty")
	ErrKeyTooLong              = errors.New("key length exceeds maximum limit")
	ErrEntryTooLarge           = errors.New("entry size exceeds shard capacity")
)
