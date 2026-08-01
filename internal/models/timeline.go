package models

import "time"

// TimelineEntry 是事件处理过程中的一条可持久化记录。
type TimelineEntry struct {
	ID          int64       `json:"id"`
	IncidentID  string      `json:"incident_id"`
	EventType   string      `json:"event_type"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Payload     interface{} `json:"payload,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}
