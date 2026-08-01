package incident

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"watchtower-lite/internal/agent"
	"watchtower-lite/internal/autofix"
	"watchtower-lite/internal/config"
	"watchtower-lite/internal/models"
	"watchtower-lite/internal/ws"

	"go.uber.org/zap"
)

// Manager owns incident state. All mutations pass through its lock so API
// reads and WebSocket serialization never race with diagnosis goroutines.
type Manager struct {
	mu           sync.RWMutex
	incidents    map[string]*models.Incident
	correlations map[string]string
	watchers     map[string]context.CancelFunc
	stateMachine *StateMachine
	notifier     *ws.Hub
	agentCli     *agent.Client
	autoFix      *autofix.Engine
	logger       *zap.Logger
	afterFixWait time.Duration
	store        IncidentStore
}

// IncidentStore 让事件管理器独立于具体数据库实现，测试时也可传入内存实现。
type IncidentStore interface {
	SaveIncident(*models.Incident) error
	LoadIncidents() ([]*models.Incident, error)
	AppendTimeline(*models.TimelineEntry) error
	ListTimeline(string) ([]models.TimelineEntry, error)
}

func NewManager(notifier *ws.Hub, agentCli *agent.Client, autoFix *autofix.Engine, logger *zap.Logger, stores ...IncidentStore) *Manager {
	observeSeconds, err := strconv.Atoi(config.Get("AUTOFIX_OBSERVE_SECONDS", "5"))
	if err != nil || observeSeconds < 0 {
		observeSeconds = 5
	}
	manager := &Manager{
		incidents:    make(map[string]*models.Incident),
		correlations: make(map[string]string),
		watchers:     make(map[string]context.CancelFunc),
		stateMachine: NewStateMachine(),
		notifier:     notifier,
		agentCli:     agentCli,
		autoFix:      autoFix,
		logger:       logger,
		afterFixWait: time.Duration(observeSeconds) * time.Second,
	}
	if len(stores) > 0 {
		manager.store = stores[0]
		if stored, loadErr := manager.store.LoadIncidents(); loadErr != nil {
			logger.Warn("加载历史事件失败", zap.Error(loadErr))
		} else {
			for _, item := range stored {
				manager.incidents[item.ID] = item
			}
			logger.Info("已恢复历史事件", zap.Int("count", len(stored)))
		}
	}
	return manager
}

// OnAlert correlates alerts for the same service and category into one active incident.
func (m *Manager) OnAlert(alert *models.Alert) {
	key := alert.Service + ":" + alert.Category()

	m.mu.Lock()
	if id, ok := m.correlations[key]; ok {
		if current, exists := m.incidents[id]; exists && !isTerminal(current.Status) {
			current.AddAlert(alert)
			snapshot := cloneIncident(current)
			m.mu.Unlock()
			m.persist(snapshot)
			m.record(snapshot.ID, "incident_updated", "关联重复告警", "同类告警已聚合到当前事件。", alert)
			m.notifier.BroadcastEvent("incident_updated", snapshot)
			return
		}
	}

	incident := models.NewIncident(alert)
	m.incidents[incident.ID] = incident
	m.correlations[key] = incident.ID
	snapshot := cloneIncident(incident)
	m.mu.Unlock()
	m.persist(snapshot)
	m.record(snapshot.ID, "new_incident", "收到新告警", snapshot.Description, snapshot.RootAlert)

	m.logger.Info("new incident received",
		zap.String("id", incident.ID),
		zap.String("service", incident.Service),
		zap.String("severity", string(incident.Severity)),
	)
	m.notifier.BroadcastEvent("new_incident", snapshot)

	switch incident.Severity {
	case models.SeverityP0, models.SeverityP1:
		go m.startDiagnosis(incident)
	case models.SeverityP2, models.SeverityP3:
		go m.observeBeforeDiagnosis(incident, 10*time.Minute)
	}
}

// OnNewIncident remains for callers that already created an incident.
func (m *Manager) OnNewIncident(incident *models.Incident) {
	if incident != nil && incident.RootAlert != nil {
		m.OnAlert(incident.RootAlert)
	}
}

func (m *Manager) startDiagnosis(incident *models.Incident) {
	snapshot, err := m.transition(incident, models.StatusDiagnosing)
	if err != nil {
		m.logger.Warn("cannot start diagnosis", zap.Error(err))
		return
	}
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)

	if m.agentCli == nil {
		m.escalate(incident, "Brain 服务不可用")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	diagnosis, err := m.agentCli.DiagnoseStream(ctx, snapshot, func(step *models.DiagnosisStep) {
		m.record(incident.ID, "diagnosis_step", fmt.Sprintf("诊断步骤 %d", step.StepNumber), step.Action, step)
		m.notifier.BroadcastEvent("diagnosis_step", map[string]interface{}{
			"incident_id": incident.ID,
			"step_number": step.StepNumber,
			"thought":     step.Thought,
			"action":      step.Action,
			"observation": step.Observation,
			"latency_ms":  step.LatencyMs,
		})
	})
	if err != nil || diagnosis == nil {
		if err == nil {
			err = fmt.Errorf("brain returned no final diagnosis")
		}
		m.logger.Error("diagnosis failed", zap.Error(err), zap.String("incident_id", incident.ID))
		m.escalate(incident, fmt.Sprintf("Agent 诊断失败: %v", err))
		return
	}

	m.mu.Lock()
	incident.Diagnosis = diagnosis
	incident.Conclusion = fmt.Sprintf("诊断结论：%s。处置建议：%s", diagnosis.Summary, diagnosis.SuggestedFix)
	m.mu.Unlock()
	snapshot, err = m.transition(incident, models.StatusDiagnosed)
	if err != nil {
		m.logger.Warn("cannot complete diagnosis", zap.Error(err))
		return
	}
	m.notifier.BroadcastEvent("diagnosis_complete", diagnosis)
	m.record(incident.ID, "diagnosis_complete", "诊断完成", diagnosis.Summary, diagnosis)
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)

	canFix, _ := m.autoFix.CanAutoFix(diagnosis)
	if diagnosis.AutoFixable && canFix {
		go m.startAutoFix(incident, diagnosis)
		return
	}
	m.escalate(incident, diagnosis.Summary)
}

func (m *Manager) observeBeforeDiagnosis(incident *models.Incident, duration time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.watchers[incident.ID] = cancel
	m.mu.Unlock()

	timer := time.NewTimer(duration)
	defer timer.Stop()
	defer func() {
		m.mu.Lock()
		delete(m.watchers, incident.ID)
		m.mu.Unlock()
	}()

	select {
	case <-timer.C:
		m.startDiagnosis(incident)
	case <-ctx.Done():
		m.logger.Info("observation cancelled", zap.String("incident_id", incident.ID))
	}
}

func (m *Manager) startAutoFix(incident *models.Incident, diagnosis *models.Diagnosis) {
	snapshot, err := m.transition(incident, models.StatusFixing)
	if err != nil {
		m.logger.Warn("cannot start auto fix", zap.Error(err))
		return
	}
	m.setConclusion(incident, fmt.Sprintf("模型诊断已完成，系统已批准自动修复：%s。正在执行模拟修复。", diagnosis.Summary))
	snapshot, _ = m.snapshot(incident)
	m.notifier.BroadcastEvent("autofix_started", snapshot)
	m.record(incident.ID, "autofix_started", "开始自动修复", diagnosis.SuggestedFix, diagnosis)
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)

	_, plan := m.autoFix.CanAutoFix(diagnosis)
	plan.SetTarget(incident.Service)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.autoFix.Execute(ctx, plan); err != nil {
		var executionErr *autofix.ExecutionError
		if errors.As(err, &executionErr) && executionErr.RolledBack {
			m.record(incident.ID, "autofix_rollback", "自动修复失败并已回滚", err.Error(), plan)
		}
		m.escalate(incident, fmt.Sprintf("自动修复失败: %v", err))
		return
	}

	snapshot, err = m.transition(incident, models.StatusFixed)
	if err != nil {
		m.logger.Warn("cannot mark auto fix complete", zap.Error(err))
		return
	}
	m.setConclusion(incident, fmt.Sprintf("自动修复执行成功。根因：%s。系统正在观察修复效果。", diagnosis.RootCause))
	snapshot, _ = m.snapshot(incident)
	m.notifier.BroadcastEvent("autofix_completed", snapshot)
	m.record(incident.ID, "autofix_completed", "自动修复执行完成", incident.Conclusion, snapshot)
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)
	go m.observeAfterFix(incident, m.afterFixWait)
}

func (m *Manager) observeAfterFix(incident *models.Incident, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	<-timer.C

	// The current executor is a simulator. A real executor must verify metrics
	// and health checks before this transition.
	snapshot, err := m.transition(incident, models.StatusResolved)
	if err != nil {
		m.logger.Warn("cannot resolve incident", zap.Error(err))
		return
	}
	m.setConclusion(incident, fmt.Sprintf("最终结论：自动修复和观察验证均已完成，事件恢复正常。根因：%s。", incident.Diagnosis.RootCause))
	snapshot, _ = m.snapshot(incident)
	m.notifier.BroadcastEvent("incident_resolved", snapshot)
	m.record(incident.ID, "incident_resolved", "事件恢复正常", incident.Conclusion, snapshot)
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)
}

func (m *Manager) escalate(incident *models.Incident, reason string) {
	m.setConclusion(incident, fmt.Sprintf("最终结论：系统未满足安全自动修复条件，已转交人工处理。原因：%s", reason))
	snapshot, err := m.transition(incident, models.StatusEscalated)
	if err != nil {
		m.logger.Warn("cannot escalate incident", zap.Error(err))
		return
	}
	m.notifier.BroadcastEvent("incident_escalated", map[string]interface{}{
		"incident": snapshot,
		"reason":   reason,
	})
	m.record(incident.ID, "incident_escalated", "事件转交人工", incident.Conclusion, map[string]string{"reason": reason})
	m.notifier.BroadcastEvent("incident_status_changed", snapshot)
}

func (m *Manager) setConclusion(incident *models.Incident, conclusion string) {
	m.mu.Lock()
	incident.Conclusion = conclusion
	snapshot := cloneIncident(incident)
	m.mu.Unlock()
	m.persist(snapshot)
}

func (m *Manager) snapshot(incident *models.Incident) (*models.Incident, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.incidents[incident.ID]; !ok {
		return nil, false
	}
	return cloneIncident(incident), true
}

func (m *Manager) transition(incident *models.Incident, to models.IncidentStatus) (*models.Incident, error) {
	m.mu.Lock()
	if err := m.stateMachine.Transition(incident, to); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	snapshot := cloneIncident(incident)
	m.mu.Unlock()
	m.persist(snapshot)
	m.record(snapshot.ID, "status_changed", "状态更新", fmt.Sprintf("事件状态已更新为%s。", statusTitle(to)), map[string]string{"status": string(to)})
	return snapshot, nil
}

func statusTitle(status models.IncidentStatus) string {
	labels := map[models.IncidentStatus]string{
		models.StatusNew: "新建", models.StatusDiagnosing: "诊断中", models.StatusDiagnosed: "已诊断",
		models.StatusFixing: "自动修复中", models.StatusFixed: "修复完成，观察中",
		models.StatusResolved: "已解决", models.StatusEscalated: "已转人工", models.StatusFalseAlarm: "误报",
	}
	if label := labels[status]; label != "" {
		return label
	}
	return string(status)
}

func (m *Manager) persist(incident *models.Incident) {
	if m.store != nil && incident != nil {
		if err := m.store.SaveIncident(incident); err != nil {
			m.logger.Warn("保存事件失败", zap.String("incident_id", incident.ID), zap.Error(err))
		}
	}
}

func (m *Manager) record(incidentID, eventType, title, description string, payload interface{}) {
	if m.store == nil {
		return
	}
	entry := &models.TimelineEntry{IncidentID: incidentID, EventType: eventType, Title: title, Description: description, Payload: payload, CreatedAt: time.Now()}
	if err := m.store.AppendTimeline(entry); err != nil {
		m.logger.Warn("保存事件时间轴失败", zap.String("incident_id", incidentID), zap.Error(err))
	}
}

func (m *Manager) Timeline(id string) ([]models.TimelineEntry, error) {
	if m.store == nil {
		return []models.TimelineEntry{}, nil
	}
	return m.store.ListTimeline(id)
}

func (m *Manager) GetIncident(id string) (*models.Incident, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	incident, ok := m.incidents[id]
	if !ok {
		return nil, false
	}
	return cloneIncident(incident), true
}

func (m *Manager) ListIncidents() []*models.Incident {
	m.mu.RLock()
	result := make([]*models.Incident, 0, len(m.incidents))
	for _, incident := range m.incidents {
		result = append(result, cloneIncident(incident))
	}
	m.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func cloneIncident(source *models.Incident) *models.Incident {
	if source == nil {
		return nil
	}
	copyValue := *source
	copyValue.RelatedAlerts = append([]*models.Alert(nil), source.RelatedAlerts...)
	return &copyValue
}

func isTerminal(status models.IncidentStatus) bool {
	return status == models.StatusResolved ||
		status == models.StatusFalseAlarm ||
		status == models.StatusEscalated
}
