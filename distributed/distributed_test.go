package distributed

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPPool(t *testing.T) {
	pool := NewHTTPPool("http://localhost:8001")
	pool.Set("http://localhost:8001", "http://localhost:8002", "http://localhost:8003")

	peers := pool.GetAll()
	if len(peers) != 3 {
		t.Errorf("Expected 3 peers, got %d", len(peers))
	}
}

func TestPickPeer(t *testing.T) {
	pool := NewHTTPPool("http://localhost:8001")
	pool.Set("http://localhost:8001", "http://localhost:8002", "http://localhost:8003")

	peer, ok := pool.PickPeer("some-key")
	if !ok {
		t.Error("Expected to pick a peer")
	}

	if peer == nil {
		t.Error("Expected non-nil peer getter")
	}
}

func TestGroupGet(t *testing.T) {
	db := map[string]string{
		"Tom":  "630",
		"Jack": "589",
		"Sam":  "567",
	}

	loadCounts := make(map[string]int, len(db))

	g := NewGroup("scores", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		if v, ok := db[key]; ok {
			loadCounts[key]++
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}))

	for k, v := range db {
		if view, err := g.Get(k); err != nil || view.String() != v {
			t.Fatalf("failed to get value of %s", k)
		}
		if _, err := g.Get(k); err != nil || loadCounts[k] > 1 {
			t.Fatalf("cache %s miss", k)
		}
	}

	if _, err := g.Get("unknown"); err == nil {
		t.Fatalf("should get error for unknown key")
	}
}

func TestGroupWithPeers(t *testing.T) {
	var peerGetter *httpGetter

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("test-value-from-peer"))
	}))
	defer server.Close()

	pool := NewHTTPPool(server.URL)
	pool.Set(server.URL)

	g := NewGroup("test-group", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	}))
	g.RegisterPeers(pool)

	peerGetter = &httpGetter{baseURL: server.URL + DefaultBasePath}

	_, err := peerGetter.Get("test-group", "test-key")
	if err != nil {
		t.Fatalf("failed to get from peer: %v", err)
	}
}

func TestByteView(t *testing.T) {
	v := ByteView{b: []byte("test")}

	if v.Len() != 4 {
		t.Errorf("Expected length 4, got %d", v.Len())
	}

	if v.String() != "test" {
		t.Errorf("Expected 'test', got '%s'", v.String())
	}

	slice := v.ByteSlice()
	if string(slice) != "test" {
		t.Errorf("Expected 'test', got '%s'", string(slice))
	}
}

func TestGetterFunc(t *testing.T) {
	f := GetterFunc(func(key string) ([]byte, error) {
		return []byte("value-" + key), nil
	})

	val, err := f.Get("test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if string(val) != "value-test" {
		t.Errorf("Expected 'value-test', got '%s'", string(val))
	}
}

func TestGetGroup(t *testing.T) {
	g1 := NewGroup("group1", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		return []byte("value"), nil
	}))

	g2 := GetGroup("group1")
	if g2 != g1 {
		t.Error("Expected same group")
	}

	g3 := GetGroup("nonexistent")
	if g3 != nil {
		t.Error("Expected nil for nonexistent group")
	}
}

func TestGroupName(t *testing.T) {
	g := NewGroup("test-name", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		return []byte("value"), nil
	}))

	if g.Name() != "test-name" {
		t.Errorf("Expected 'test-name', got '%s'", g.Name())
	}
}

func TestDistributedCacheFlow(t *testing.T) {
	db := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group := GetGroup("test")
		if group == nil {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}
		view, err := group.Get(r.URL.Path[len(DefaultBasePath+"test/"):])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(view.ByteSlice())
	}))
	defer server1.Close()

	g := NewGroup("test", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		time.Sleep(10 * time.Millisecond)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("key not found: %s", key)
	}))

	pool := NewHTTPPool(server1.URL)
	pool.Set(server1.URL)
	g.RegisterPeers(pool)

	view, err := g.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get key1: %v", err)
	}
	if view.String() != "value1" {
		t.Errorf("Expected 'value1', got '%s'", view.String())
	}

	view2, err := g.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get cached key1: %v", err)
	}
	if view2.String() != "value1" {
		t.Errorf("Expected 'value1' from cache, got '%s'", view2.String())
	}
}
