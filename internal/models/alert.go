package models

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Severity 告警严重级别
type Severity string

const (
	SeverityP0 Severity = "P0" // 紧急
	SeverityP1 Severity = "P1" // 重要
	SeverityP2 Severity = "P2" // 一般
	SeverityP3 Severity = "P3" // 提示
)

// Alert 原始告警
type Alert struct {
	ID          string            `json:"id"`
	Source      string            `json:"source"`     // prometheus / custom
	Service     string            `json:"service"`    // 服务名
	AlertName   string            `json:"alert_name"` // 告警规则名
	Severity    Severity          `json:"severity"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Value       float64           `json:"value"`
	Timestamp   time.Time         `json:"timestamp"`
}

// NewAlert 创建新告警
func NewAlert(source, service, alertName string, severity Severity, description string) *Alert {
	return &Alert{
		ID:          uuid.New().String(),
		Source:      source,
		Service:     service,
		AlertName:   alertName,
		Severity:    severity,
		Description: description,
		Labels:      make(map[string]string),
		Timestamp:   time.Now(),
	}
}

// Fingerprint 生成告警指纹（用于去重）
func (a *Alert) Fingerprint() string {
	h := sha256.New()
	h.Write([]byte(a.Source))
	h.Write([]byte(a.Service))
	h.Write([]byte(a.AlertName))
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := a.Labels[k]
		h.Write([]byte(k + v))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// Category 返回告警分类（用于聚合）
func (a *Alert) Category() string {
	// 基于告警名提取分类
	switch a.AlertName {
	case "HighCPU", "HighMemory":
		return "resource"
	case "HighLatency", "HighErrorRate":
		return "performance"
	case "PodCrash", "CrashLoop", "OOMKilled":
		return "availability"
	case "DiskFull":
		return "storage"
	case "ConnTimeout":
		return "dependency"
	default:
		return "unknown"
	}
}
