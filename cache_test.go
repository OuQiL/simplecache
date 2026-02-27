package simplecache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	cache, err := New(DefaultConfig(time.Minute))
	if err != nil {
		t.Fatalf("创建缓存失败: %v", err)
	}
	defer cache.Close()

	if cache == nil {
		t.Fatal("缓存实例不应为空")
	}

	stats := cache.Stats()
	if stats.ShardCount != 256 {
		t.Errorf("期望分片数为256，实际为%d", stats.ShardCount)
	}
}

func TestSetAndGet(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	key := "test-key"
	value := []byte("test-value")

	err := cache.Set(key, value)
	if err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	got, found := cache.Get(key)
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

	cache, _ := New(config)
	defer cache.Close()

	key := "expire-key"
	value := []byte("expire-value")

	err := cache.SetWithTTL(key, value, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("SetWithTTL 失败: %v", err)
	}

	got, found := cache.Get(key)
	if !found {
		t.Fatal("条目应该存在")
	}
	if string(got) != string(value) {
		t.Errorf("期望值 %s，实际得到 %s", value, got)
	}

	time.Sleep(150 * time.Millisecond)

	_, found = cache.Get(key)
	if found {
		t.Fatal("过期条目不应该被找到")
	}
}

func TestDelete(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	key := "delete-key"
	value := []byte("delete-value")

	cache.Set(key, value)

	_, found := cache.Get(key)
	if !found {
		t.Fatal("条目应该存在")
	}

	cache.Delete(key)

	_, found = cache.Get(key)
	if found {
		t.Fatal("删除后条目不应该存在")
	}
}

func TestExists(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	key := "exists-key"

	if cache.Exists(key) {
		t.Fatal("key不应该存在")
	}

	cache.Set(key, []byte("value"))

	if !cache.Exists(key) {
		t.Fatal("key应该存在")
	}
}

func TestLen(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	if cache.Len() != 0 {
		t.Errorf("初始长度应该为0，实际为%d", cache.Len())
	}

	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	if cache.Len() != 10 {
		t.Errorf("期望长度为10，实际为%d", cache.Len())
	}
}

func TestClear(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("清空后长度应该为0，实际为%d", cache.Len())
	}
}

func TestConcurrent(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

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
				err := cache.Set(key, value)
				if err != nil {
					t.Errorf("Set 失败: %v", err)
					return
				}

				got, found := cache.Get(key)
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
	if cache.Len() != expectedTotal {
		t.Errorf("期望总条目数为%d，实际为%d", expectedTotal, cache.Len())
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

	cache, _ := New(config)
	defer cache.Close()

	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))

	cache.Delete("key1")

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

	cache, _ := New(config)
	defer cache.Close()

	stats := cache.Stats()
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

func TestInvalidTTL(t *testing.T) {
	config := DefaultConfig(0)

	_, err := New(config)
	if err != ErrInvalidTTL {
		t.Errorf("期望错误 ErrInvalidTTL，实际得到: %v", err)
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
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	err := cache.Set("", []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("期望错误 ErrEmptyKey，实际得到: %v", err)
	}
}

func TestSetKeyTooLong(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	longKey := make([]byte, 65536)
	for i := range longKey {
		longKey[i] = 'a'
	}

	err := cache.Set(string(longKey), []byte("value"))
	if err != ErrKeyTooLong {
		t.Errorf("期望错误 ErrKeyTooLong，实际得到: %v", err)
	}
}

func TestValueCopy(t *testing.T) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	originalValue := []byte("original")
	cache.Set("key", originalValue)

	got, _ := cache.Get("key")
	got[0] = 'X'

	got2, _ := cache.Get("key")
	if string(got2) != "original" {
		t.Errorf("Get应该返回副本，原值不应被修改，实际为%s", got2)
	}
}

func BenchmarkSet(b *testing.B) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}
}

func BenchmarkGet(b *testing.B) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(fmt.Sprintf("key-%d", i%1000))
	}
}

func BenchmarkConcurrentSet(b *testing.B) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
			i++
		}
	})
}

func BenchmarkConcurrentGet(b *testing.B) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()

	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Get(fmt.Sprintf("key-%d", i%1000))
			i++
		}
	})
}

func BenchmarkConcurrentMixed(b *testing.B) {
	cache, _ := New(DefaultConfig(time.Hour))
	defer cache.Close()
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				cache.Set(key, []byte("value"))
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}
