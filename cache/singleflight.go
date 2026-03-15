package cache

import "sync"

type call struct {
	val []byte
	b   bool
	wg  sync.WaitGroup
}

type sf struct {
	mu sync.Mutex
	m  map[string]*call
}

func (sf *sf) Do(key string, hash uint64, fn func() ([]byte, bool)) ([]byte, bool) {
	sf.mu.Lock()
	if sf.m == nil {
		sf.m = make(map[string]*call)
	}

	// map里有值（有wg）等待结果
	c, ok := sf.m[key]
	if ok {
		sf.mu.Unlock()
		c.wg.Wait()
		return c.val, c.b
	}

	// 没有则新开
	sf.m[key] = &call{}
	c = sf.m[key]
	c.wg.Add(1)
	sf.mu.Unlock()

	//get
	defer func() {
		if r := recover(); r != nil {
			c.val, c.b = nil, false
		}
		c.wg.Done()
	}()
	c.val, c.b = fn()

	// delete map record
	sf.mu.Lock()
	delete(sf.m, key)
	sf.mu.Unlock()

	return c.val, c.b
}
