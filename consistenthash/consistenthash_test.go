package consistenthash

import (
	"strconv"
	"testing"
)

func TestHashing(t *testing.T) {
	hash := New(3, func(key []byte) uint64 {
		i, _ := strconv.Atoi(string(key))
		return uint64(i)
	})

	hash.Add("6", "4", "2")

	testCases := map[string]string{
		"2":  "2",
		"11": "2",
		"23": "4",
		"27": "2",
	}

	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}

	hash.Add("8")

	testCases["27"] = "8"

	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}
}

func TestConsistency(t *testing.T) {
	hash1 := New(1, nil)
	hash2 := New(1, nil)

	hash1.Add("Bill", "Bob", "Bonny")
	hash2.Add("Bob", "Bonny", "Bill")

	if hash1.Get("Ben") != hash2.Get("Ben") {
		t.Errorf("Fetching 'Ben' from both hashes should yield same result")
	}
}

func TestRemove(t *testing.T) {
	hash := New(3, func(key []byte) uint64 {
		i, _ := strconv.Atoi(string(key))
		return uint64(i)
	})

	hash.Add("6", "4", "2")

	if hash.Get("23") != "4" {
		t.Errorf("Expected '4', got '%s'", hash.Get("23"))
	}

	hash.Remove("4")

	if hash.Get("23") != "6" {
		t.Errorf("Expected '6' after removing '4', got '%s'", hash.Get("23"))
	}
}

func TestIsEmpty(t *testing.T) {
	hash := New(3, nil)

	if !hash.IsEmpty() {
		t.Error("Expected hash to be empty")
	}

	hash.Add("node1")

	if hash.IsEmpty() {
		t.Error("Expected hash to not be empty")
	}
}

func TestDefaultHash(t *testing.T) {
	hash := New(50, nil)

	hash.Add("node1", "node2", "node3")

	node := hash.Get("some-key")
	if node == "" {
		t.Error("Expected non-empty node")
	}

	validNodes := map[string]bool{"node1": true, "node2": true, "node3": true}
	if !validNodes[node] {
		t.Errorf("Expected valid node, got '%s'", node)
	}
}
