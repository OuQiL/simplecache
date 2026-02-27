package simplecache

import (
	"sync"

	"github.com/cespare/xxhash/v2"
)

var hasherPool = sync.Pool{
	New: func() interface{} {
		return xxhash.New() // 当池为空时，用这个函数新建
	},
}

func hashKey(key string) uint64 {
	h := hasherPool.Get().(*xxhash.Digest)
	h.Reset()
	h.WriteString(key)
	sum := h.Sum64()
	hasherPool.Put(h)
	return sum
}
