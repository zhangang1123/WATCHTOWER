package incident

import (
	"fmt"

	"watchtower-lite/internal/models"
)

// StateMachine 事件状态机
type StateMachine struct {
	transitions map[models.IncidentStatus][]models.IncidentStatus
}

// NewStateMachine 创建状态机
func NewStateMachine() *StateMachine {
	return &StateMachine{
		transitions: map[models.IncidentStatus][]models.IncidentStatus{
			models.StatusNew: {
				models.StatusDiagnosing,
				models.StatusFalseAlarm,
			},
			models.StatusDiagnosing: {
				models.StatusDiagnosed,
				models.StatusEscalated,
			},
			models.StatusDiagnosed: {
				models.StatusFixing,
				models.StatusEscalated,
				models.StatusResolved,
			},
			models.StatusFixing: {
				models.StatusFixed,
				models.StatusEscalated,
			},
			models.StatusFixed: {
				models.StatusResolved,
				models.StatusEscalated,
			},
			models.StatusEscalated: {
				models.StatusResolved,
			},
		},
	}
}

// CanTransition 检查状态转换是否合法
func (sm *StateMachine) CanTransition(from, to models.IncidentStatus) bool {
	allowed, ok := sm.transitions[from]
	if !ok {
		return false
	}
	for _, status := range allowed {
		if status == to {
			return true
		}
	}
	return false
}

// Transition 执行状态转换，非法转换返回错误
func (sm *StateMachine) Transition(incident *models.Incident, to models.IncidentStatus) error {
	if !sm.CanTransition(incident.Status, to) {
		return fmt.Errorf("invalid state transition: %s -> %s", incident.Status, to)
	}
	incident.UpdateStatus(to)
	return nil
}
