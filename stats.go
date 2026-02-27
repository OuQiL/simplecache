package simplecache

type Stats struct {
	TotalEntries int
	ShardCount   int
}

func (c *Cache) Stats() Stats {
	return Stats{
		TotalEntries: c.Len(),
		ShardCount:   len(c.shards),
	}
}
