package gateway

import (
	"watchtower-lite/internal/models"
)

// Normalizer 告警标准化器
type Normalizer struct{}

// NewNormalizer 创建标准化器
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// FromPrometheus 将 Prometheus AlertManager 格式转为内部告警
func (n *Normalizer) FromPrometheus(payload PrometheusWebhookPayload) []*models.Alert {
	var alerts []*models.Alert
	for _, pa := range payload.Alerts {
		if pa.Status == "resolved" {
			continue
		}
		severity := models.SeverityP3
		if s, ok := pa.Labels["severity"]; ok {
			candidate := models.Severity(s)
			switch candidate {
			case models.SeverityP0, models.SeverityP1, models.SeverityP2, models.SeverityP3:
				severity = candidate
			}
		}

		alert := models.NewAlert(
			"prometheus",
			pa.Labels["service"],
			pa.Labels["alertname"],
			severity,
			pa.Annotations["summary"],
		)
		alert.Labels = pa.Labels
		alerts = append(alerts, alert)
	}
	return alerts
}
