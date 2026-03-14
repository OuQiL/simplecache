# SimpleCache

## 安装

```bash
go get github.com/OuQiL/simplecache/cache
```

## 快速开始

```go
package main

import (
	"fmt"

	"time"

	"github.com/OuQiL/simplecache/cache"
)

func main() {
	// 创建缓存实例
	config := cache.DefaultConfig(time.Minute)
	cache, err := cache.New(config)
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	// 写入数据
	cache.Set("user:1", []byte("Alice"))

	// 读取数据
	value, found := cache.Get("user:1")
	if found {
		fmt.Println(string(value)) // Alice
	}
	fmt.Println("1:", cache.Stats()) // {TotalEntries: 1, ShardCount: 256}
	// 设置带 TTL 的数据（100ms 后过期）
	cache.SetWithTTL("token:abc", []byte("xyz"), 100*time.Millisecond)
	value, found = cache.Get("token:abc")
	if found {
		fmt.Println(string(value)) // Alice
	}
	fmt.Println("2:", cache.Stats()) // {2 256}
	// 删除数据
	cache.Delete("user:1")
	fmt.Println("3:", cache.Stats()) // {1 256}
	time.Sleep(100 * time.Millisecond)
	value, found = cache.Get("token:abc")
	if !found {
		fmt.Println("not found") // Alice
	}
}

```

## 配置说明

```go
type Config struct {
    ShardCount       int           // 分片数量，必须是 2 的幂次方（默认 256）
    DefaultTTL       time.Duration // 默认过期时间（默认 0，永不过期）
    CleanInterval    time.Duration // 清理过期条目间隔（默认 1 分钟）
    OnEvicted        func(key string, value []byte) // 淘汰回调
    MaxBytesPerShard int           // 每个分片最大字节数（默认 4MB）
}
```

### 使用淘汰回调

```go
config := simplecache.Config{
    ShardCount:       256,
    DefaultTTL:       time.Hour,
    CleanInterval:    time.Minute,
    MaxBytesPerShard: 4 * 1024 * 1024,
    OnEvicted: func(key string, value []byte) {
        fmt.Printf("淘汰 key: %s, value: %s\n", key, value)
    },
}
```

## 架构设计

### 分片策略

```
Cache (缓存管理器)
├── shards []*shard  (256个分片)
├── shardMask       (位运算快速定位)
└── entryCount      (原子计数)
```

每个 key 通过 `hash(key) & mask` 快速定位到对应分片，不同分片可以并行读写。

### 内部结构

```
shard (单个分片)
├── data      []byte  (预分配环形缓冲区)
├── write     uint32  (写入位置指针)
├── oldest    uint32  (淘汰位置指针)
└── entries   map[hash]pos (索引)
```

### 数据格式

```
┌──────────┬──────────┬─────────────┬──────────┬──────────┐
│ keyLen   │ valueLen │  expireAt   │   key    │  value   │
│ (2字节)  │ (4字节)  │  (8字节)    │ (变长)   │ (变长)   │
└──────────┴──────────┴─────────────┴──────────┴──────────┘
0          2          6             14          14+keyLen
```

## 适用场景

- ✅ 读多写多的高并发缓存
- ✅ 短期数据缓存（如 token、会话）
- ✅ 计数器缓存

- ❌ 需要持久化的数据
- ❌ 需要遍历所有 key 的场景

## 与其他库对比

| 特性 | SimpleCache | BigCache | FreeCache |
|------|-------------|-----------|------------|
| 分片 | 256 | 1024 | 32 |
| 压缩 | ❌ | ✅ | ❌ |
| 持久化 | ❌ | ✅ | ✅ |
| GC 压力 | 低 | 中 | 低 |
| 代码复杂度 | 低 | 高 | 中 |

## 性能测试

```bash
go test -bench=. -benchmem
```

## 注意事项

1. **数据仅存于内存**，重启后会丢失
2. **key 不能为空**，最大长度为 65535 字节
3. **单条数据不能超过分片大小**
4. **建议设置合理的 TTL**，配合 CleanInterval 清理过期数据

## 许可证

MIT License
