package models

import "testing"

func TestFingerprintIsIndependentOfLabelInsertionOrder(t *testing.T) {
	first := NewAlert("prometheus", "payment", "HighCPU", SeverityP1, "cpu high")
	first.Labels["region"] = "east"
	first.Labels["pod"] = "payment-1"

	second := NewAlert("prometheus", "payment", "HighCPU", SeverityP1, "cpu high")
	second.Labels["pod"] = "payment-1"
	second.Labels["region"] = "east"

	if first.Fingerprint() != second.Fingerprint() {
		t.Fatalf("equal alerts produced different fingerprints: %s != %s", first.Fingerprint(), second.Fingerprint())
	}
}
