package gateway

import (
	"testing"

	"watchtower-lite/internal/models"
)

func TestPriorityQueueOrdersAndBoundsAlerts(t *testing.T) {
	queue := NewPriorityQueue(2)
	defer queue.Close()

	p3 := models.NewAlert("test", "svc", "Info", models.SeverityP3, "info")
	p0 := models.NewAlert("test", "svc", "Down", models.SeverityP0, "down")
	if !queue.Push(p3) || !queue.Push(p0) {
		t.Fatal("queue rejected alerts before reaching its limit")
	}
	if queue.Push(models.NewAlert("test", "svc", "Extra", models.SeverityP1, "extra")) {
		t.Fatal("queue accepted an alert beyond its configured limit")
	}
	if got := queue.Pop(); got != p0 {
		t.Fatalf("expected P0 first, got %s", got.Severity)
	}
	if got := queue.Pop(); got != p3 {
		t.Fatalf("expected P3 second, got %s", got.Severity)
	}
}
