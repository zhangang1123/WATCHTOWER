package models

import (
	"time"

	"github.com/google/uuid"
)

// IncidentStatus 事件状态
type IncidentStatus string

const (
	StatusNew        IncidentStatus = "new"         // 新事件
	StatusDiagnosing IncidentStatus = "diagnosing"  // 诊断中
	StatusDiagnosed  IncidentStatus = "diagnosed"   // 已诊断
	StatusFixing     IncidentStatus = "fixing"      // 修复中
	StatusFixed      IncidentStatus = "fixed"       // 已修复，观察中
	StatusResolved   IncidentStatus = "resolved"    // 已解决
	StatusEscalated  IncidentStatus = "escalated"   // 已升级人工
	StatusFalseAlarm IncidentStatus = "false_alarm" // 误报
)

// Incident 运维事件（由一个或多个告警聚合而成）
type Incident struct {
	ID            string         `json:"id"`
	Service       string         `json:"service"`
	Severity      Severity       `json:"severity"`
	Status        IncidentStatus `json:"status"`
	Description   string         `json:"description"`
	Conclusion    string         `json:"conclusion"`
	RootAlert     *Alert         `json:"root_alert"`
	RelatedAlerts []*Alert       `json:"related_alerts"`
	Diagnosis     *Diagnosis     `json:"diagnosis,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ResolvedAt    *time.Time     `json:"resolved_at,omitempty"`
	MTTRSeconds   int64          `json:"mttr_seconds,omitempty"` // 修复耗时（秒）
}

// NewIncident 从告警创建事件
func NewIncident(alert *Alert) *Incident {
	now := time.Now()
	return &Incident{
		ID:            uuid.New().String(),
		Service:       alert.Service,
		Severity:      alert.Severity,
		Status:        StatusNew,
		Description:   alert.Description,
		Conclusion:    "告警已接收，等待系统完成诊断并给出处置结论。",
		RootAlert:     alert,
		RelatedAlerts: []*Alert{alert},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// AddAlert 添加关联告警
func (i *Incident) AddAlert(alert *Alert) {
	i.RelatedAlerts = append(i.RelatedAlerts, alert)
	// 更新严重度为最高级别
	if alert.Severity == SeverityP0 || (alert.Severity == SeverityP1 && i.Severity != SeverityP0) {
		i.Severity = alert.Severity
	}
}

// UpdateStatus 更新状态
func (i *Incident) UpdateStatus(status IncidentStatus) {
	i.Status = status
	i.UpdatedAt = time.Now()
	if status == StatusResolved || status == StatusFalseAlarm {
		now := time.Now()
		i.ResolvedAt = &now
		i.MTTRSeconds = int64(now.Sub(i.CreatedAt).Seconds())
	}
}
