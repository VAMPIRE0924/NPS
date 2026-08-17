package cache

import (
	"sync"
	"testing"
)

func TestCacheClearKeepsCacheUsable(t *testing.T) {
	c := New(2)
	c.Add("a", 1)
	c.Clear()
	if c.Len() != 0 {
		t.Fatal("cache was not cleared")
	}
	c.Add("b", 2)
	if value, ok := c.Get("b"); !ok || value.(int) != 2 {
		t.Fatal("cache is unusable after clear")
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := New(32)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(key int) {
			defer wg.Done()
			c.Add(key, key)
			_, _ = c.Get(key)
			if key%3 == 0 {
				c.Remove(key)
			}
		}(i)
	}
	wg.Wait()
	if c.Len() > 32 {
		t.Fatalf("cache exceeded limit: %d", c.Len())
	}
}
