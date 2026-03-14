package consistenthash

import (
	"sort"
	"strconv"
	"sync"

	"github.com/cespare/xxhash/v2"
)

type Hash func(data []byte) uint64

type Map struct {
	sync.RWMutex
	hash     Hash
	replicas int
	keys     []uint64
	hashMap  map[uint64]string
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[uint64]string),
	}
	if m.hash == nil {
		m.hash = defaultHash
	}
	return m
}

func defaultHash(data []byte) uint64 {
	h := xxhash.New()
	h.Write(data)
	return h.Sum64()
}

func (m *Map) Add(keys ...string) {
	m.Lock()
	defer m.Unlock()

	for _, key := range keys {
		for i := 0; i < m.replicas; i++ {
			hash := m.hash([]byte(strconv.Itoa(i) + key))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key
		}
	}
	sort.Slice(m.keys, func(i, j int) bool {
		return m.keys[i] < m.keys[j]
	})
}

func (m *Map) Get(key string) string {
	m.RLock()
	defer m.RUnlock()

	if len(m.keys) == 0 {
		return ""
	}

	hash := m.hash([]byte(key))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})

	if idx == len(m.keys) {
		idx = 0
	}

	return m.hashMap[m.keys[idx]]
}

func (m *Map) Remove(key string) {
	m.Lock()
	defer m.Unlock()

	for i := 0; i < m.replicas; i++ {
		hash := m.hash([]byte(strconv.Itoa(i) + key))
		delete(m.hashMap, hash)
	}

	var newKeys []uint64
	for _, h := range m.keys {
		if _, ok := m.hashMap[h]; ok {
			newKeys = append(newKeys, h)
		}
	}
	m.keys = newKeys
}

func (m *Map) IsEmpty() bool {
	m.RLock()
	defer m.RUnlock()
	return len(m.keys) == 0
}
