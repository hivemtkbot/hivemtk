package cache

import (
	"runtime"
	"testing"
	"time"
)

func TestShutdownAll_ClosesAllRegistered(t *testing.T) {
	before := runtime.NumGoroutine()

	caches := make([]*MemoryCache, 0, 20)
	for i := 0; i < 20; i++ {
		caches = append(caches, NewMemoryCache())
	}

	afterCreate := runtime.NumGoroutine()

	ShutdownAll()
	time.Sleep(50 * time.Millisecond)

	afterShutdown := runtime.NumGoroutine()

	if afterShutdown > before+5 {
		t.Errorf("ShutdownAll 没能清理所有 goroutine: before=%d afterCreate=%d afterShutdown=%d",
			before, afterCreate, afterShutdown)
	}

	for _, c := range caches {
		c.Close()
	}
}
