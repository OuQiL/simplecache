# SimpleCache

## 快速开始

### 1.本地缓存使用

```bash
go get github.com/OuQiL/simplecache/cache
```

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

### 2.分布式缓存使用

```bash
go get github.com/OuQiL/simplecache/distributed
```

```go
package main

import (
	"fmt"
	"github.com/OuQiL/simplecache/distributed"
)

func aFunc(key string) ([]byte, error) {
	return []byte("getter"), nil
}

func main() {
	// Init
	group := distributed.NewGroup("name", 10<<20, distributed.GetterFunc(aFunc))
	pool := distributed.NewHTTPPool("http://localhost:8001")
	pool.Set("http://localhost:8001", "http://localhost:8002")
	group.RegisterPeers(pool)
	// 命中回调
	val, _ := group.Get("Key1")
	fmt.Println(val) //"getter"
	// Set
	group.Set("Key2", []byte("value-true"))
	val, _ = group.Get("Key2")
	fmt.Println(val) //value-true"
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

## 性能测试

### grpc测试

```bash
go run benchmark/bench.go -mode=grpc-bench -peers=localhost:50051,localhost:50052,localhost:50053 -concurrency=1000 -requests=100000
```

```bash
=== gRPC 压测报告 ===
总请求数: 100000
成功请求: 100000 (100.00%)
失败请求: 0 (0.00%)
总耗时: 1.1908068s
QPS: 83976.68
平均延迟: 9.819578ms
=== 测试完成 ===
```

### HTTP测试

```bash
go run benchmark/bench.go -mode bench -peers localhost:8001,localhost:8002,localhost:8003 -concurrency 1000 -requests 50000
```

```bash
开始压测，并发数: 1000, 请求数: 50000
测试节点: [localhost:8001 localhost:8002 localhost:8003]
预热中...

=== 压测报告 ===
总请求数: 50000
成功请求: 49658 (99.32%)
失败请求: 342 (0.68%)
总耗时: 12.8404748s
QPS: 3893.94
平均延迟: 168.81537ms
=== 测试完成 ===
```

### 单点性能测试

```bash
go run benchmark/bench.go -mode bench -peers localhost:8001 -concurrency 2000 -requests 50000
开始压测，并发数: 2000, 请求数: 50000
测试节点: [localhost:8001]
预热中...

=== 压测报告 ===
总请求数: 50000
成功请求: 48962 (97.92%)
失败请求: 1038 (2.08%)
总耗时: 8.7577248s
QPS: 5709.25
平均延迟: 347.066404ms
=== 测试完成 ===
```

## 注意事项

1. **数据仅存于内存**，重启后会丢失
2. **key 不能为空**，最大长度为 65535 字节
3. **单条数据不能超过分片大小**
4. **建议设置合理的 TTL**，配合 CleanInterval 清理过期数据

