package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"watchtower-lite/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler 告警接入处理器
type Handler struct {
	normalizer     *Normalizer
	deduplicator   *Deduplicator
	silencer       *Silencer
	queue          *PriorityQueue
	httpClient     *http.Client
	mockControlURL string
	logger         *zap.Logger
	onAlert        func(*models.Alert) // 告警处理回调
}

// NewHandler 创建处理器
func NewHandler(logger *zap.Logger, mockControlURL string, onAlert func(*models.Alert)) *Handler {
	h := &Handler{
		normalizer:     NewNormalizer(),
		deduplicator:   NewDeduplicator(),
		silencer:       NewSilencer(),
		queue:          NewPriorityQueue(1000),
		httpClient:     &http.Client{Timeout: 3 * time.Second},
		mockControlURL: mockControlURL,
		logger:         logger,
		onAlert:        onAlert,
	}
	go h.consume()
	return h
}

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

var supportedScenarios = map[string]bool{
	"oom_kill": true, "high_cpu": true, "high_latency": true,
	"disk_full": true, "conn_timeout": true, "crash_loop": true,
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/webhook/prometheus", h.handlePrometheusWebhook)
	r.POST("/webhook/custom", h.handleCustomWebhook)
	r.POST("/api/v1/mock/trigger", h.handleMockTrigger)
	r.POST("/api/v1/mock/storm", h.handleAlertStorm)
}

// handlePrometheusWebhook 处理 Prometheus AlertManager Webhook
func (h *Handler) handlePrometheusWebhook(c *gin.Context) {
	var payload PrometheusWebhookPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 标准化为内部告警格式
	alerts := h.normalizer.FromPrometheus(payload)
	for _, alert := range alerts {
		h.processAlert(alert)
	}

	c.JSON(http.StatusOK, gin.H{"received": len(alerts)})
}

// handleCustomWebhook 处理自定义告警
func (h *Handler) handleCustomWebhook(c *gin.Context) {
	var req struct {
		Service     string            `json:"service" binding:"required"`
		AlertName   string            `json:"alert_name" binding:"required"`
		Severity    models.Severity   `json:"severity" binding:"required"`
		Description string            `json:"description" binding:"required"`
		Labels      map[string]string `json:"labels"`
		Value       float64           `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !serviceNamePattern.MatchString(req.Service) || !validSeverity(req.Severity) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service name or severity"})
		return
	}
	alert := models.NewAlert("custom", req.Service, req.AlertName, req.Severity, req.Description)
	alert.Labels = req.Labels
	alert.Value = req.Value
	h.processAlert(alert)
	c.JSON(http.StatusOK, gin.H{"received": 1})
}

// handleMockTrigger 手动触发模拟告警
func (h *Handler) handleMockTrigger(c *gin.Context) {
	var req struct {
		Scenario      string `json:"scenario" binding:"required"`
		Service       string `json:"service" binding:"required"`
		RepairOutcome string `json:"repair_outcome"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !supportedScenarios[req.Scenario] || !serviceNamePattern.MatchString(req.Service) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported scenario or invalid service name"})
		return
	}
	if err := h.registerMockScenario(req.Scenario, req.Service, req.RepairOutcome); err != nil {
		h.logger.Error("failed to register mock scenario", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "mock infrastructure is unavailable"})
		return
	}

	alert := generateMockAlert(req.Scenario, req.Service)
	h.processAlert(alert)

	c.JSON(http.StatusOK, gin.H{
		"message": "mock alert triggered",
		"alert":   alert,
	})
}

// handleAlertStorm 在单机中生成可控的告警风暴，用于展示去重、排队和吞吐量。
func (h *Handler) handleAlertStorm(c *gin.Context) {
	var req struct {
		Count          int     `json:"count"`
		DuplicateRatio float64 `json:"duplicate_ratio"`
		Service        string  `json:"service"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Count < 1 {
		req.Count = 500
	}
	if req.Count > 5000 {
		req.Count = 5000
	}
	if req.DuplicateRatio < 0 || req.DuplicateRatio > 1 {
		req.DuplicateRatio = 0.8
	}
	if req.Service == "" {
		req.Service = "storm-demo"
	}
	if !serviceNamePattern.MatchString(req.Service) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务名称不合法"})
		return
	}

	started := time.Now()
	stats := map[string]int{"accepted": 0, "deduplicated": 0, "silenced": 0, "dropped": 0}
	uniqueCount := int(float64(req.Count) * (1 - req.DuplicateRatio))
	if uniqueCount < 1 {
		uniqueCount = 1
	}
	for index := 0; index < req.Count; index++ {
		service := req.Service + "-0"
		if index < uniqueCount {
			service = fmt.Sprintf("%s-%d", req.Service, index)
		}
		alert := models.NewAlert("storm", service, "SyntheticStorm", models.SeverityP3, "模拟告警风暴中的重复低优先级告警")
		stats[h.processAlert(alert)]++
	}
	elapsed := time.Since(started)
	elapsedNanos := elapsed.Nanoseconds()
	if elapsedNanos < 1 {
		elapsedNanos = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"requested": req.Count, "accepted": stats["accepted"], "deduplicated": stats["deduplicated"],
		"silenced": stats["silenced"], "dropped": stats["dropped"], "elapsed_ms": elapsed.Milliseconds(),
		"throughput_per_second": float64(req.Count) * float64(time.Second) / float64(elapsedNanos),
	})
}

// processAlert 处理单条告警
func (h *Handler) processAlert(alert *models.Alert) string {
	// 1. 去重检查
	if h.deduplicator.IsDuplicate(alert) {
		h.logger.Info("alert deduplicated", zap.String("fingerprint", alert.Fingerprint()))
		return "deduplicated"
	}

	// 2. 静默检查
	if h.silencer.IsSilenced(alert) {
		h.logger.Info("alert silenced", zap.String("service", alert.Service))
		return "silenced"
	}

	// 3. 加入优先级队列
	if !h.queue.Push(alert) {
		h.logger.Warn("alert queue full, alert dropped", zap.String("service", alert.Service))
		return "dropped"
	}

	h.logger.Info("alert received",
		zap.String("service", alert.Service),
		zap.String("severity", string(alert.Severity)),
		zap.String("alert_name", alert.AlertName),
	)
	return "accepted"
}

func (h *Handler) consume() {
	for {
		alert := h.queue.Pop()
		if alert == nil {
			return
		}
		if h.onAlert != nil {
			h.onAlert(alert)
		}
	}
}

func (h *Handler) registerMockScenario(scenario, service, repairOutcome string) error {
	body, err := json.Marshal(gin.H{"scenario": scenario, "service": service, "repair_outcome": repairOutcome})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, h.mockControlURL+"/mock/scenarios", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mock control returned %s", resp.Status)
	}
	return nil
}

func (h *Handler) Close() {
	h.queue.Close()
}

func validSeverity(severity models.Severity) bool {
	switch severity {
	case models.SeverityP0, models.SeverityP1, models.SeverityP2, models.SeverityP3:
		return true
	default:
		return false
	}
}

// generateMockAlert 生成模拟告警
func generateMockAlert(scenario, service string) *models.Alert {
	var alert *models.Alert
	switch scenario {
	case "oom_kill":
		alert = models.NewAlert("mock", service, "OOMKilled", models.SeverityP0,
			fmt.Sprintf("%s 服务的 Pod 因内存耗尽被系统终止", service))
	case "high_cpu":
		alert = models.NewAlert("mock", service, "HighCPU", models.SeverityP1,
			fmt.Sprintf("%s 服务的 CPU 使用率持续超过 90%%", service))
	case "high_latency":
		alert = models.NewAlert("mock", service, "HighLatency", models.SeverityP1,
			fmt.Sprintf("%s 服务的 P99 请求延迟超过 500 毫秒", service))
	case "disk_full":
		alert = models.NewAlert("mock", service, "DiskFull", models.SeverityP2,
			fmt.Sprintf("%s 服务所在节点的磁盘使用率超过 85%%", service))
	case "conn_timeout":
		alert = models.NewAlert("mock", service, "ConnTimeout", models.SeverityP0,
			fmt.Sprintf("%s 服务访问下游依赖时连接超时", service))
	case "crash_loop":
		alert = models.NewAlert("mock", service, "CrashLoop", models.SeverityP0,
			fmt.Sprintf("%s 服务的 Pod 反复崩溃并持续重启", service))
	default:
		alert = models.NewAlert("mock", service, "Unknown", models.SeverityP3,
			fmt.Sprintf("%s 服务出现未知告警", service))
	}
	return alert
}

// PrometheusWebhookPayload Prometheus AlertManager Webhook 格式
type PrometheusWebhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []struct {
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations"`
		StartsAt     string            `json:"startsAt"`
		EndsAt       string            `json:"endsAt"`
		GeneratorURL string            `json:"generatorURL"`
		Fingerprint  string            `json:"fingerprint"`
	} `json:"alerts"`
}
