package store

import (
	"testing"

	"watchtower-lite/internal/models"
)

func TestSQLiteRoundTrip(t *testing.T) {
	store, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alert := models.NewAlert("test", "payment", "HighCPU", models.SeverityP1, "CPU 使用率过高")
	incident := models.NewIncident(alert)
	if err := store.SaveIncident(incident); err != nil {
		t.Fatal(err)
	}
	entry := &models.TimelineEntry{IncidentID: incident.ID, EventType: "new_incident", Title: "收到告警", Description: incident.Description}
	if err := store.AppendTimeline(entry); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadIncidents()
	if err != nil || len(loaded) != 1 || loaded[0].ID != incident.ID {
		t.Fatalf("unexpected incidents: %#v, %v", loaded, err)
	}
	timeline, err := store.ListTimeline(incident.ID)
	if err != nil || len(timeline) != 1 || timeline[0].Title != "收到告警" {
		t.Fatalf("unexpected timeline: %#v, %v", timeline, err)
	}
}
