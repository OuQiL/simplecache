package main

import (
	"fmt"
	"github.com/OuQiL/simplecache/distributed"
)

func aFunc(key string) ([]byte, error) {
	return []byte("getter"), nil
}

func main() {
	group := distributed.NewGroup("name", 10<<20, distributed.GetterFunc(aFunc))
	pool := distributed.NewHTTPPool("http://localhost:8001")
	pool.Set("http://localhost:8001", "http://localhost:8002")
	group.RegisterPeers(pool)
	val, _ := group.Get("Key1")
	fmt.Println(val) //"getter"
	group.Set("Key2", []byte("value-true"))
	val, _ = group.Get("Key2")
	fmt.Println(val) //value-true"
}
