package mock

import (
	"fmt"
	"sync"
	"time"
)

const (
	StateFiring     = "firing"
	StateFixing     = "fixing"
	StateRecovered  = "recovered"
	StateFailed     = "failed"
	StateRolledBack = "rolled_back"
)

// ScenarioData 是指标、事件、日志和修复状态共享的唯一数据源。
type ScenarioData struct {
	Service       string             `json:"service"`
	Scenario      string             `json:"scenario"`
	State         string             `json:"state"`
	RepairOutcome string             `json:"repair_outcome"`
	Metrics       map[string]float64 `json:"metrics"`
	Events        []K8sEvent         `json:"events"`
	Logs          []string           `json:"logs"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type ScenarioRegistry struct {
	mu        sync.RWMutex
	scenarios map[string]*ScenarioData
}

func NewScenarioRegistry() *ScenarioRegistry {
	return &ScenarioRegistry{scenarios: make(map[string]*ScenarioData)}
}

func (r *ScenarioRegistry) Register(service, scenario, outcome string) *ScenarioData {
	if outcome != "failure" {
		outcome = "success"
	}
	data := generateScenarioData(service, scenario)
	data.State = StateFiring
	data.RepairOutcome = outcome
	r.mu.Lock()
	r.scenarios[service] = data
	r.mu.Unlock()
	return cloneScenario(data)
}

func (r *ScenarioRegistry) Snapshot(service string) (*ScenarioData, bool) {
	r.mu.RLock()
	data, ok := r.scenarios[service]
	if ok {
		data = cloneScenario(data)
	}
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return dataForState(data), true
}

func (r *ScenarioRegistry) ApplyAction(service, action string) (*ScenarioData, error) {
	r.mu.Lock()
	data, ok := r.scenarios[service]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("service scenario not found")
	}
	if action == "rollback" {
		data.State = StateRolledBack
		data.UpdatedAt = time.Now()
		result := cloneScenario(data)
		r.mu.Unlock()
		return dataForState(result), nil
	}
	data.State = StateFixing
	data.UpdatedAt = time.Now()
	outcome := data.RepairOutcome
	r.mu.Unlock()

	go func() {
		time.Sleep(700 * time.Millisecond)
		r.mu.Lock()
		defer r.mu.Unlock()
		current, exists := r.scenarios[service]
		if !exists || current.State != StateFixing {
			return
		}
		if outcome == "failure" {
			current.State = StateFailed
		} else {
			current.State = StateRecovered
		}
		current.UpdatedAt = time.Now()
	}()
	result, _ := r.Snapshot(service)
	return result, nil
}

func cloneScenario(source *ScenarioData) *ScenarioData {
	if source == nil {
		return nil
	}
	copyValue := *source
	copyValue.Metrics = make(map[string]float64, len(source.Metrics))
	for key, value := range source.Metrics {
		copyValue.Metrics[key] = value
	}
	copyValue.Events = append([]K8sEvent(nil), source.Events...)
	copyValue.Logs = append([]string(nil), source.Logs...)
	return &copyValue
}

func dataForState(data *ScenarioData) *ScenarioData {
	switch data.State {
	case StateFixing:
		data.Events = append(data.Events, K8sEvent{Type: "正常", Reason: "正在修复", Message: "系统正在执行受控修复动作"})
		data.Logs = append(data.Logs, "信息：自动修复动作正在执行")
	case StateRecovered:
		data.Metrics["up"] = 1
		data.Metrics["cpu_usage"] = 0.42
		data.Metrics["memory_usage"] = 268435456
		data.Metrics["latency_p99"] = 0.08
		data.Metrics["error_rate"] = 0.002
		data.Events = []K8sEvent{{Type: "正常", Reason: "服务已恢复", Message: "工作负载健康检查连续通过"}}
		data.Logs = []string{"信息：服务重新启动完成", "信息：健康检查已通过", "信息：请求处理恢复正常"}
	case StateFailed:
		data.Events = append(data.Events, K8sEvent{Type: "警告", Reason: "修复验证失败", Message: "执行修复后故障仍然存在"})
		data.Logs = append(data.Logs, "错误：修复后健康检查仍未通过")
	case StateRolledBack:
		data.Events = append(data.Events, K8sEvent{Type: "正常", Reason: "已执行回滚", Message: "已恢复到修复前的模拟状态并等待人工处理"})
		data.Logs = append(data.Logs, "警告：自动修复失败，系统已执行回滚")
	}
	return data
}
