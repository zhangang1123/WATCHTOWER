package gateway

import (
	"fmt"

	"watchtower-lite/internal/models"
)

// Aggregator 告警聚合器
type Aggregator struct{}

// NewAggregator 创建聚合器
func NewAggregator() *Aggregator {
	return &Aggregator{}
}

// Aggregate 将关联告警聚合为事件
func (a *Aggregator) Aggregate(alerts []*models.Alert) []*models.Incident {
	groups := make(map[string]*models.Incident)

	for _, alert := range alerts {
		key := fmt.Sprintf("%s:%s", alert.Service, alert.Category())

		if inc, exists := groups[key]; exists {
			inc.AddAlert(alert)
		} else {
			groups[key] = models.NewIncident(alert)
		}
	}

	result := make([]*models.Incident, 0, len(groups))
	for _, inc := range groups {
		result = append(result, inc)
	}
	return result
}
