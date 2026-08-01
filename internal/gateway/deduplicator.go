package gateway

import (
	"sync"
	"time"

	"watchtower-lite/internal/models"
)

// Deduplicator 告警去重器（内存版）
type Deduplicator struct {
	seen map[string]time.Time
	mu   sync.RWMutex
}

// NewDeduplicator 创建去重器
func NewDeduplicator() *Deduplicator {
	d := &Deduplicator{
		seen: make(map[string]time.Time),
	}
	// 启动清理 goroutine
	go d.cleanup()
	return d
}

// IsDuplicate 检查是否为重复告警（5 分钟内相同指纹视为重复）
func (d *Deduplicator) IsDuplicate(alert *models.Alert) bool {
	key := alert.Fingerprint()

	d.mu.Lock()
	defer d.mu.Unlock()

	if lastSeen, ok := d.seen[key]; ok {
		if time.Since(lastSeen) < 5*time.Minute {
			return true
		}
	}

	d.seen[key] = time.Now()
	return false
}

// cleanup 定期清理过期记录
func (d *Deduplicator) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.mu.Lock()
		for key, ts := range d.seen {
			if time.Since(ts) > 10*time.Minute {
				delete(d.seen, key)
			}
		}
		d.mu.Unlock()
	}
}
