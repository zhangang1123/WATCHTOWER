package gateway

import (
	"watchtower-lite/internal/models"
)

// Silencer 告警静默器
type Silencer struct {
	rules []SilenceRule
}

// SilenceRule 静默规则
type SilenceRule struct {
	Service   string
	AlertName string
	Reason    string
}

// NewSilencer 创建静默器（预置一些已知误报规则）
func NewSilencer() *Silencer {
	return &Silencer{
		rules: []SilenceRule{
			// 可以在这里添加已知的误报规则
			// {Service: "test-service", AlertName: "TestAlert", Reason: "测试环境告警"},
		},
	}
}

// IsSilenced 检查告警是否被静默
func (s *Silencer) IsSilenced(alert *models.Alert) bool {
	for _, rule := range s.rules {
		if (rule.Service == "" || rule.Service == alert.Service) &&
			(rule.AlertName == "" || rule.AlertName == alert.AlertName) {
			return true
		}
	}
	return false
}
