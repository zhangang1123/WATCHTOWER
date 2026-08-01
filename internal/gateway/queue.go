package gateway

import (
	"container/heap"
	"sync"

	"watchtower-lite/internal/models"
)

// PriorityQueue 告警优先级队列
type PriorityQueue struct {
	items  alertHeap
	mu     sync.Mutex
	ready  *sync.Cond
	limit  int
	closed bool
}

// NewPriorityQueue 创建优先级队列
func NewPriorityQueue(limit int) *PriorityQueue {
	pq := &PriorityQueue{
		items: make(alertHeap, 0),
		limit: limit,
	}
	pq.ready = sync.NewCond(&pq.mu)
	heap.Init(&pq.items)
	return pq
}

// Push 加入队列
func (pq *PriorityQueue) Push(alert *models.Alert) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if pq.closed || (pq.limit > 0 && pq.items.Len() >= pq.limit) {
		return false
	}
	heap.Push(&pq.items, alert)
	pq.ready.Signal()
	return true
}

// Pop 取出最高优先级告警
func (pq *PriorityQueue) Pop() *models.Alert {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	for pq.items.Len() == 0 && !pq.closed {
		pq.ready.Wait()
	}
	if pq.closed && pq.items.Len() == 0 {
		return nil
	}
	return heap.Pop(&pq.items).(*models.Alert)
}

// Close 停止队列并唤醒消费者。
func (pq *PriorityQueue) Close() {
	pq.mu.Lock()
	pq.closed = true
	pq.mu.Unlock()
	pq.ready.Broadcast()
}

// Len 返回队列长度
func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.items.Len()
}

// alertHeap 告警堆实现
type alertHeap []*models.Alert

func (h alertHeap) Len() int { return len(h) }

func (h alertHeap) Less(i, j int) bool {
	// P0 > P1 > P2 > P3
	severityOrder := map[models.Severity]int{
		models.SeverityP0: 4,
		models.SeverityP1: 3,
		models.SeverityP2: 2,
		models.SeverityP3: 1,
	}
	return severityOrder[h[i].Severity] > severityOrder[h[j].Severity]
}

func (h alertHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *alertHeap) Push(x interface{}) {
	*h = append(*h, x.(*models.Alert))
}

func (h *alertHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
