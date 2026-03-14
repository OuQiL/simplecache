package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	c, err := New(DefaultConfig(time.Minute))
	if err != nil {
		t.Fatalf("创建缓存失败: %v", err)
	}
	defer c.Close()

	if c == nil {
		t.Fatal("缓存实例不应为空")
	}

	stats := c.Stats()
	if stats.ShardCount != 256 {
		t.Errorf("期望分片数为256，实际为%d", stats.ShardCount)
	}
}

func TestSetAndGet(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	key := "test-key"
	value := []byte("test-value")

	err := c.Set(key, value)
	if err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	got, found := c.Get(key)
	if !found {
		t.Fatal("应该找到key")
	}

	if string(got) != string(value) {
		t.Errorf("期望值 %s，实际得到 %s", value, got)
	}
}

func TestSetWithTTL(t *testing.T) {
	config := DefaultConfig(time.Hour)
	config.CleanInterval = 100 * time.Millisecond

	c, _ := New(config)
	defer c.Close()

	key := "expire-key"
	value := []byte("expire-value")

	err := c.SetWithTTL(key, value, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("SetWithTTL 失败: %v", err)
	}

	got, found := c.Get(key)
	if !found {
		t.Fatal("条目应该存在")
	}
	if string(got) != string(value) {
		t.Errorf("期望值 %s，实际得到 %s", value, got)
	}

	time.Sleep(150 * time.Millisecond)

	_, found = c.Get(key)
	if found {
		t.Fatal("过期条目不应该被找到")
	}
}

func TestDelete(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	key := "delete-key"
	value := []byte("delete-value")

	c.Set(key, value)

	_, found := c.Get(key)
	if !found {
		t.Fatal("条目应该存在")
	}

	c.Delete(key)

	_, found = c.Get(key)
	if found {
		t.Fatal("删除后条目不应该存在")
	}
}

func TestExists(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	key := "exists-key"

	if c.Exists(key) {
		t.Fatal("key不应该存在")
	}

	c.Set(key, []byte("value"))

	if !c.Exists(key) {
		t.Fatal("key应该存在")
	}
}

func TestLen(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	if c.Len() != 0 {
		t.Errorf("初始长度应该为0，实际为%d", c.Len())
	}

	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	if c.Len() != 10 {
		t.Errorf("期望长度为10，实际为%d", c.Len())
	}
}

func TestClear(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("清空后长度应该为0，实际为%d", c.Len())
	}
}

func TestConcurrent(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	concurrency := 100
	entriesPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entriesPerGoroutine; j++ {
				key := fmt.Sprintf("goroutine-%d-key-%d", id, j)
				value := []byte(fmt.Sprintf("value-%d-%d", id, j))
				err := c.Set(key, value)
				if err != nil {
					t.Errorf("Set 失败: %v", err)
					return
				}

				got, found := c.Get(key)
				if !found {
					t.Errorf("并发写入后应该能读取到key: %s", key)
					return
				}
				if string(got) != string(value) {
					t.Errorf("并发读取值不匹配，期望 %s，实际 %s", value, got)
				}
			}
		}(i)
	}

	wg.Wait()

	expectedTotal := concurrency * entriesPerGoroutine
	if c.Len() != expectedTotal {
		t.Errorf("期望总条目数为%d，实际为%d", expectedTotal, c.Len())
	}
}

func TestOnEvicted(t *testing.T) {
	evictedKeys := make([]string, 0)
	evictedValues := make([][]byte, 0)

	config := DefaultConfig(time.Hour)
	config.OnEvicted = func(key string, value []byte) {
		evictedKeys = append(evictedKeys, key)
		evictedValues = append(evictedValues, value)
	}

	c, _ := New(config)
	defer c.Close()

	c.Set("key1", []byte("value1"))
	c.Set("key2", []byte("value2"))

	c.Delete("key1")

	if len(evictedKeys) != 1 {
		t.Fatalf("期望1个驱逐回调，实际有%d个", len(evictedKeys))
	}

	if evictedKeys[0] != "key1" {
		t.Errorf("期望驱逐key1，实际驱逐%s", evictedKeys[0])
	}

	if string(evictedValues[0]) != "value1" {
		t.Errorf("期望驱逐value1，实际驱逐%s", evictedValues[0])
	}
}

func TestCustomShardCount(t *testing.T) {
	config := DefaultConfig(time.Hour)
	config.ShardCount = 64

	c, _ := New(config)
	defer c.Close()

	stats := c.Stats()
	if stats.ShardCount != 64 {
		t.Errorf("期望分片数为64，实际为%d", stats.ShardCount)
	}
}

func TestInvalidShardCount(t *testing.T) {
	config := DefaultConfig(time.Hour)
	config.ShardCount = 100

	_, err := New(config)
	if err != ErrInvalidShardCount {
		t.Errorf("期望错误 ErrInvalidShardCount，实际得到: %v", err)
	}
}

func TestInvalidMaxBytesPerShard(t *testing.T) {
	config := DefaultConfig(time.Hour)
	config.MaxBytesPerShard = 0

	_, err := New(config)
	if err != ErrInvalidMaxBytesPerShard {
		t.Errorf("期望错误 ErrInvalidMaxBytesPerShard，实际得到: %v", err)
	}
}

func TestSetEmptyKey(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	err := c.Set("", []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("期望错误 ErrEmptyKey，实际得到: %v", err)
	}
}

func TestSetKeyTooLong(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	longKey := make([]byte, 65536)
	for i := range longKey {
		longKey[i] = 'a'
	}

	err := c.Set(string(longKey), []byte("value"))
	if err != ErrKeyTooLong {
		t.Errorf("期望错误 ErrKeyTooLong，实际得到: %v", err)
	}
}

func TestValueCopy(t *testing.T) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	originalValue := []byte("original")
	c.Set("key", originalValue)

	got, _ := c.Get("key")
	got[0] = 'X'

	got2, _ := c.Get("key")
	if string(got2) != "original" {
		t.Errorf("Get应该返回副本，原值不应被修改，实际为%s", got2)
	}
}

func BenchmarkSet(b *testing.B) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}
}

func BenchmarkGet(b *testing.B) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("key-%d", i%1000))
	}
}

func BenchmarkConcurrentSet(b *testing.B) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
			i++
		}
	})
}

func BenchmarkConcurrentGet(b *testing.B) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()

	for i := 0; i < 1000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(fmt.Sprintf("key-%d", i%1000))
			i++
		}
	})
}

func BenchmarkConcurrentMixed(b *testing.B) {
	c, _ := New(DefaultConfig(time.Hour))
	defer c.Close()
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				c.Set(key, []byte("value"))
			} else {
				c.Get(key)
			}
			i++
		}
	})
}
