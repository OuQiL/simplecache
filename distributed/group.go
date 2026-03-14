package distributed

import (
	"errors"
	"sync"

	"github.com/OuQiL/simplecache/cache"
)

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

var (
	ErrNotFound = errors.New("key not found")
)

type ByteView struct {
	b []byte
}

func (v ByteView) Len() int {
	return len(v.b)
}

func (v ByteView) ByteSlice() []byte {
	return v.b
}

func (v ByteView) String() string {
	return string(v.b)
}

type Group struct {
	name      string
	getter    Getter
	mainCache *cache.Cache
	peers     PeerPicker
}

type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()

	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: createCache(cacheBytes),
	}
	groups[name] = g
	return g
}

func createCache(cacheBytes int64) *cache.Cache {
	if cacheBytes <= 0 {
		return nil
	}

	config := cache.DefaultConfig(0)
	config.MaxBytesPerShard = int(cacheBytes / int64(config.ShardCount))
	if config.MaxBytesPerShard < 1024 {
		config.MaxBytesPerShard = 1024
	}

	c, err := cache.New(config)
	if err != nil {
		return nil
	}
	return c
}

func GetGroup(name string) *Group {
	mu.RLock()
	defer mu.RUnlock()
	return groups[name]
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, errors.New("key is required")
	}

	if g.mainCache != nil {
		if v, ok := g.mainCache.Get(key); ok {
			return ByteView{b: v}, nil
		}
	}

	return g.load(key)
}

func (g *Group) load(key string) (value ByteView, err error) {
	if g.peers != nil {
		if peer, ok := g.peers.PickPeer(key); ok {
			value, err = g.getFromPeer(peer, key)
			if err == nil {
				return value, nil
			}
		}
	}

	return g.getLocally(key)
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
}

func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}

	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {
	if g.mainCache != nil {
		g.mainCache.Set(key, value.ByteSlice())
	}
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func (g *Group) Name() string {
	return g.name
}
