package autofix

import (
	"testing"

	"watchtower-lite/internal/models"

	"go.uber.org/zap"
)

func TestCanAutoFixUsesStructuredFixType(t *testing.T) {
	engine := NewEngine(zap.NewNop())
	diagnosis := &models.Diagnosis{
		IncidentID:   "inc-1",
		FixType:      "restart_pod",
		SuggestedFix: "human-readable instructions",
		Confidence:   0.93,
	}
	canFix, plan := engine.CanAutoFix(diagnosis)
	if !canFix {
		t.Fatal("expected structured restart_pod action to be auto-fixable")
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "restart_pod" {
		t.Fatalf("unexpected generated plan: %#v", plan)
	}
}
