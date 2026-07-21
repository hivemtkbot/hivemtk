package browser

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"
)

type MessageDedup interface {
	IsDuplicate(ctx context.Context, platform, chatID, messageID, content string) bool
}

type InMemoryDedup struct {
	mu   sync.RWMutex
	seen map[string]time.Time
	ttl  time.Duration
}

func NewInMemoryDedup(ttl time.Duration) *InMemoryDedup {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	d := &InMemoryDedup{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
	go d.cleanupLoop()
	return d
}

func (d *InMemoryDedup) key(platform, chatID, messageID, content string) string {
	raw := fmt.Sprintf("%s:%s:%s:%s", platform, chatID, messageID, content)
	return fmt.Sprintf("%x", md5.Sum([]byte(raw)))[:16]
}

func (d *InMemoryDedup) IsDuplicate(ctx context.Context, platform, chatID, messageID, content string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	k := d.key(platform, chatID, messageID, content)
	if _, exists := d.seen[k]; exists {
		return true
	}
	d.seen[k] = time.Now()
	return false
}

func (d *InMemoryDedup) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		cutoff := time.Now().Add(-d.ttl)
		for k, t := range d.seen {
			if t.Before(cutoff) {
				delete(d.seen, k)
			}
		}
		d.mu.Unlock()
	}
}
