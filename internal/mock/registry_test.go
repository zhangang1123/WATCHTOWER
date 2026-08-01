package mock

import (
	"testing"
	"time"
)

func TestScenarioRegistryRepairLifecycle(t *testing.T) {
	registry := NewScenarioRegistry()
	registry.Register("payment", "crash_loop", "success")
	if _, err := registry.ApplyAction("payment", "restart_pod"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(850 * time.Millisecond)
	data, ok := registry.Snapshot("payment")
	if !ok || data.State != StateRecovered || data.Metrics["up"] != 1 {
		t.Fatalf("expected recovered scenario, got %#v", data)
	}
}

func TestScenarioRegistryFailedRepairCanRollback(t *testing.T) {
	registry := NewScenarioRegistry()
	registry.Register("payment", "crash_loop", "failure")
	if _, err := registry.ApplyAction("payment", "restart_pod"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(850 * time.Millisecond)
	data, _ := registry.Snapshot("payment")
	if data.State != StateFailed {
		t.Fatalf("expected failed state, got %s", data.State)
	}
	data, err := registry.ApplyAction("payment", "rollback")
	if err != nil || data.State != StateRolledBack {
		t.Fatalf("expected rolled back state, got %#v, %v", data, err)
	}
}
