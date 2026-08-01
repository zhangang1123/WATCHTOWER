package gateway

import (
	"testing"

	"watchtower-lite/internal/models"
)

func TestNormalizerSkipsResolvedAndDefaultsUnknownSeverity(t *testing.T) {
	var payload PrometheusWebhookPayload
	payload.Alerts = append(payload.Alerts,
		struct {
			Status       string            `json:"status"`
			Labels       map[string]string `json:"labels"`
			Annotations  map[string]string `json:"annotations"`
			StartsAt     string            `json:"startsAt"`
			EndsAt       string            `json:"endsAt"`
			GeneratorURL string            `json:"generatorURL"`
			Fingerprint  string            `json:"fingerprint"`
		}{Status: "resolved", Labels: map[string]string{"service": "old", "alertname": "Old"}},
		struct {
			Status       string            `json:"status"`
			Labels       map[string]string `json:"labels"`
			Annotations  map[string]string `json:"annotations"`
			StartsAt     string            `json:"startsAt"`
			EndsAt       string            `json:"endsAt"`
			GeneratorURL string            `json:"generatorURL"`
			Fingerprint  string            `json:"fingerprint"`
		}{
			Status:      "firing",
			Labels:      map[string]string{"service": "payment", "alertname": "HighCPU", "severity": "critical"},
			Annotations: map[string]string{"summary": "cpu high"},
		},
	)

	alerts := NewNormalizer().FromPrometheus(payload)
	if len(alerts) != 1 {
		t.Fatalf("expected one firing alert, got %d", len(alerts))
	}
	if alerts[0].Severity != models.SeverityP3 {
		t.Fatalf("unknown severity should default to P3, got %s", alerts[0].Severity)
	}
}
