package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"watchtower-lite/internal/models"

	_ "modernc.org/sqlite"
)

// SQLiteStore 保存事件快照和完整时间轴。快照使用 JSON，可在模型扩展字段后保持兼容。
type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if path == "" {
		path = "data/watchtower.db"
	}
	if path != ":memory:" {
		path = filepath.Clean(path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS incidents (
			id TEXT PRIMARY KEY,
			service TEXT NOT NULL,
			severity TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_incidents_updated ON incidents(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_incidents_service_status ON incidents(service, status)`,
		`CREATE TABLE IF NOT EXISTS incident_timeline (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			payload_json TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(incident_id) REFERENCES incidents(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_timeline_incident ON incident_timeline(incident_id, id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("sqlite migration: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) SaveIncident(incident *models.Incident) error {
	payload, err := json.Marshal(incident)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO incidents(id, service, severity, status, created_at, updated_at, payload_json)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET service=excluded.service, severity=excluded.severity,
		status=excluded.status, updated_at=excluded.updated_at, payload_json=excluded.payload_json`,
		incident.ID, incident.Service, incident.Severity, incident.Status,
		incident.CreatedAt.Format(time.RFC3339Nano), incident.UpdatedAt.Format(time.RFC3339Nano), string(payload))
	return err
}

func (s *SQLiteStore) LoadIncidents() ([]*models.Incident, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM incidents ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*models.Incident
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var incident models.Incident
		if err := json.Unmarshal([]byte(payload), &incident); err != nil {
			return nil, err
		}
		result = append(result, &incident)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) AppendTimeline(entry *models.TimelineEntry) error {
	createdAt := entry.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	payload, err := json.Marshal(entry.Payload)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`INSERT INTO incident_timeline
		(incident_id, event_type, title, description, payload_json, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		entry.IncidentID, entry.EventType, entry.Title, entry.Description, string(payload), createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	entry.ID, _ = result.LastInsertId()
	entry.CreatedAt = createdAt
	return nil
}

func (s *SQLiteStore) ListTimeline(incidentID string) ([]models.TimelineEntry, error) {
	rows, err := s.db.Query(`SELECT id, event_type, title, description, payload_json, created_at
		FROM incident_timeline WHERE incident_id=? ORDER BY id`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]models.TimelineEntry, 0)
	for rows.Next() {
		var entry models.TimelineEntry
		var payload, createdAt string
		entry.IncidentID = incidentID
		if err := rows.Scan(&entry.ID, &entry.EventType, &entry.Title, &entry.Description, &payload, &createdAt); err != nil {
			return nil, err
		}
		entry.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if payload != "" && payload != "null" {
			var value interface{}
			if json.Unmarshal([]byte(payload), &value) == nil {
				entry.Payload = value
			}
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *SQLiteStore) Close() error { return s.db.Close() }
