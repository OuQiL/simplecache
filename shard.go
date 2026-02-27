package simplecache

import (
	"encoding/binary"
	"sync"
	"time"
)

const (
	headerSize  = 14    // 头部大小（见下图）
	keyLenOff   = 0     // key长度在头部偏移量
	valueLenOff = 2     // value长度在头部偏移量
	expireAtOff = 6     // 过期时间在头部偏移量
	keyDataOff  = 14    // key数据在头部偏移量
	maxKeyLen   = 65535 // 最大key长度 256*256-1 即两字节
)

/*
┌──────────┬──────────┬─────────────┬──────────┬──────────┐
│ keyLen   │ valueLen │  expireAt   │   key    │  value   │
│ (2字节)   │(4字节)    │ (8字节)     │  (变长)   │ (变长)    │
└──────────┴──────────┴─────────────┴──────────┴──────────┘
 0          2          6             14          14+keyLen
*/

// shard 分片
// 极大的byte数组和一个map索引
type shard struct {
	sync.RWMutex
	data      []byte            //预分配
	write     uint32            //现在写入位置偏移量
	oldest    uint32            //最老位置偏移量
	entries   map[uint64]uint32 //key-hash->entry-position
	onEvicted func(key string, value []byte)
	totalSize uint32 // 总大小
}

// 构造函数
func newShard(maxBytesPerShard int, onEvicted func(key string, value []byte)) *shard {
	return &shard{
		data:      make([]byte, maxBytesPerShard),
		write:     0,
		oldest:    0,
		entries:   make(map[uint64]uint32),
		onEvicted: onEvicted,
		totalSize: uint32(maxBytesPerShard),
	}
}

func (s *shard) set(key string, hash uint64, value []byte, expireAt int64) (int, error) {
	s.Lock()
	defer s.Unlock()

	keyLen := len(key)
	valueLen := len(value)
	// 检查key是否为空
	if keyLen == 0 {
		return 0, ErrEmptyKey
	}
	// 检查key长度是否超过最大长度
	if keyLen > maxKeyLen {
		return 0, ErrKeyTooLong
	}
	// 检查entry大小是否超过最大大小
	entrySize := uint32(headerSize + keyLen + valueLen)
	if entrySize > s.totalSize {
		return 0, ErrEntryTooLarge
	}

	delta := 1
	if pos, exists := s.entries[hash]; exists {
		if s.keyMatches(pos, key) {
			// 哈希碰撞处理 删旧
			s.markDeleted(pos)
			delete(s.entries, hash)
			delta = 0
		}
	}
	//
	evicted := s.makeSpace(entrySize)
	// 写入entry
	binary.LittleEndian.PutUint16(s.data[s.write:], uint16(keyLen))
	binary.LittleEndian.PutUint32(s.data[s.write+valueLenOff:], uint32(valueLen))
	binary.LittleEndian.PutUint64(s.data[s.write+expireAtOff:], uint64(expireAt))

	copy(s.data[s.write+keyDataOff:], key)
	copy(s.data[s.write+keyDataOff+uint32(keyLen):], value)

	s.entries[hash] = s.write
	s.write += entrySize

	return delta - evicted, nil
}

// 是否哈希碰撞
func (s *shard) keyMatches(pos uint32, key string) bool {
	keyLen := int(binary.LittleEndian.Uint16(s.data[pos:]))
	storedKey := s.data[pos+keyDataOff : pos+keyDataOff+uint32(keyLen)]
	return string(storedKey) == key
}

func (s *shard) readKey(pos uint32) string {
	keyLen := int(binary.LittleEndian.Uint16(s.data[pos:]))
	return string(s.data[pos+keyDataOff : pos+keyDataOff+uint32(keyLen)])
}

func (s *shard) makeSpace(size uint32) int {
	evicted := 0
	for !s.hasSpace(size) {
		if s.evictOldest() {
			evicted++
		} else {
			break
		}
	}

	if s.write+size > s.totalSize {
		s.write = 0
	}

	return evicted
}

/*
 */
func (s *shard) hasSpace(size uint32) bool {
	if len(s.entries) == 0 {
		return s.write+size <= s.totalSize
	}

	if s.write >= s.oldest {
		if s.write+size <= s.totalSize {
			return true
		}
		return size <= s.oldest
	}

	return s.write+size <= s.oldest
}

func (s *shard) evictOldest() bool {
	if len(s.entries) == 0 {
		s.write = 0
		s.oldest = 0
		return false
	}

	keyLen := int(binary.LittleEndian.Uint16(s.data[s.oldest:]))

	for keyLen == 0 {
		s.oldest += headerSize
		if s.oldest >= s.totalSize {
			s.oldest = 0
		}
		if s.oldest == s.write {
			s.write = 0
			s.oldest = 0
			return false
		}
		keyLen = int(binary.LittleEndian.Uint16(s.data[s.oldest:]))
	}

	valueLen := int(binary.LittleEndian.Uint32(s.data[s.oldest+valueLenOff:]))
	entrySize := uint32(headerSize + keyLen + valueLen)
	key := s.readKey(s.oldest)
	hash := hashKey(key)

	if s.onEvicted != nil {
		value := s.readValue(s.oldest, keyLen, valueLen)
		s.onEvicted(key, value)
	}

	delete(s.entries, hash)

	s.oldest += entrySize
	if s.oldest >= s.totalSize {
		s.oldest = 0
	}

	return true
}

func (s *shard) markDeleted(pos uint32) {
	binary.LittleEndian.PutUint16(s.data[pos:], 0)
}

func (s *shard) readHeader(pos uint32) (keyLen int, valueLen int, expireAt int64, err error) {
	keyLen = int(binary.LittleEndian.Uint16(s.data[pos:]))
	valueLen = int(binary.LittleEndian.Uint32(s.data[pos+valueLenOff:]))
	expireAt = int64(binary.LittleEndian.Uint64(s.data[pos+expireAtOff:]))
	return
}

func (s *shard) readValue(pos uint32, keyLen, valueLen int) []byte {
	start := pos + keyDataOff + uint32(keyLen)
	value := make([]byte, valueLen)
	copy(value, s.data[start:start+uint32(valueLen)])
	return value
}

func (s *shard) get(key string, hash uint64) ([]byte, bool) {
	s.RLock()
	defer s.RUnlock()

	pos, exists := s.entries[hash]
	if !exists {
		return nil, false
	}

	if !s.keyMatches(pos, key) {
		return nil, false
	}

	keyLen, valueLen, expireAt, _ := s.readHeader(pos)
	if keyLen == 0 {
		return nil, false
	}

	if expireAt > 0 && time.Now().UnixNano() > expireAt {
		return nil, false
	}

	value := s.readValue(pos, keyLen, valueLen)
	return value, true
}

func (s *shard) delete(key string, hash uint64) int {
	s.Lock()
	defer s.Unlock()

	pos, exists := s.entries[hash]
	if !exists {
		return 0
	}

	if !s.keyMatches(pos, key) {
		return 0
	}

	keyLen, valueLen, _, _ := s.readHeader(pos)
	if s.onEvicted != nil {
		value := s.readValue(pos, keyLen, valueLen)
		s.onEvicted(key, value)
	}

	s.markDeleted(pos)
	delete(s.entries, hash)

	return -1
}

func (s *shard) count() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.entries)
}

func (s *shard) clear() int {
	s.Lock()
	defer s.Unlock()

	count := len(s.entries)

	if s.onEvicted != nil {
		for _, pos := range s.entries {
			key := s.readKey(pos)
			keyLen, valueLen, _, _ := s.readHeader(pos)
			value := s.readValue(pos, keyLen, valueLen)
			s.onEvicted(key, value)
		}
	}

	s.entries = make(map[uint64]uint32)
	s.write = 0
	s.oldest = 0

	return -count
}

func (s *shard) cleanupExpired() int {
	s.Lock()
	defer s.Unlock()

	now := time.Now().UnixNano()
	var toDelete []uint64

	for hash, pos := range s.entries {
		_, _, expireAt, _ := s.readHeader(pos)
		if expireAt > 0 && now > expireAt {
			toDelete = append(toDelete, hash)
		}
	}

	for _, hash := range toDelete {
		if pos, ok := s.entries[hash]; ok {
			key := s.readKey(pos)
			keyLen, valueLen, _, _ := s.readHeader(pos)
			if s.onEvicted != nil {
				value := s.readValue(pos, keyLen, valueLen)
				s.onEvicted(key, value)
			}
			s.markDeleted(pos)
			delete(s.entries, hash)
		}
	}

	return -len(toDelete)
}
