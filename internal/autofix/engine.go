package autofix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"watchtower-lite/internal/models"
)

// RiskLevel 风险等级
type RiskLevel int

const (
	RiskLow    RiskLevel = iota // 自动执行
	RiskMedium                  // 推送确认
	RiskHigh                    // 人工执行
)

// Engine 自动修复引擎
type Engine struct {
	riskRules []RiskRule
	logger    *zap.Logger
	mockURL   string
	httpCli   *http.Client
}

// RiskRule 风险评估规则
type RiskRule struct {
	FixType       string
	RiskLevel     RiskLevel
	AutoThreshold float64 // 自动执行的最小置信度
}

// NewEngine 创建自动修复引擎
func NewEngine(logger *zap.Logger, mockURLs ...string) *Engine {
	engine := &Engine{
		riskRules: []RiskRule{
			{FixType: "restart_pod", RiskLevel: RiskLow, AutoThreshold: 0.85},
			{FixType: "scale", RiskLevel: RiskLow, AutoThreshold: 0.90},
			{FixType: "cleanup_disk", RiskLevel: RiskMedium, AutoThreshold: 0.95},
			{FixType: "config_change", RiskLevel: RiskHigh, AutoThreshold: 1.0},
			{FixType: "traffic_switch", RiskLevel: RiskHigh, AutoThreshold: 1.0},
		},
		logger:  logger,
		httpCli: &http.Client{Timeout: 2 * time.Second},
	}
	if len(mockURLs) > 0 {
		engine.mockURL = strings.TrimRight(mockURLs[0], "/")
	}
	return engine
}

// FixPlan 修复方案
type FixPlan struct {
	IncidentID  string
	Steps       []FixStep
	RiskLevel   RiskLevel
	Rollback    []FixStep
	VerifySteps []VerifyStep
}

// FixStep 修复步骤
type FixStep struct {
	Action      string `json:"action"`
	Target      string `json:"target"`
	Description string `json:"description"`
}

// VerifyStep 验证步骤
type VerifyStep struct {
	CheckType string `json:"check_type"` // metric / log / health
	Target    string `json:"target"`
	Condition string `json:"condition"`
}

// CanAutoFix 判断是否可以自动修复
func (e *Engine) CanAutoFix(diagnosis *models.Diagnosis) (bool, *FixPlan) {
	// 基于诊断结果生成修复方案
	plan := e.generateFixPlan(diagnosis)

	// 检查风险等级和置信度
	for _, rule := range e.riskRules {
		if rule.FixType == diagnosis.FixType {
			plan.RiskLevel = rule.RiskLevel
			if diagnosis.Confidence >= rule.AutoThreshold && rule.RiskLevel == RiskLow {
				return true, plan
			}
			return false, plan
		}
	}

	// 默认不自动执行
	plan.RiskLevel = RiskHigh
	return false, plan
}

// Execute 执行修复方案
func (e *Engine) Execute(ctx context.Context, plan *FixPlan) error {
	e.logger.Info("executing fix plan",
		zap.String("incident", plan.IncidentID),
		zap.Int("risk_level", int(plan.RiskLevel)),
		zap.Int("steps", len(plan.Steps)),
	)

	for i, step := range plan.Steps {
		e.logger.Info("executing fix step",
			zap.Int("step", i+1),
			zap.String("action", step.Action),
		)
		// 实际执行逻辑（mock 版本只记录日志）
		if err := e.executeStep(ctx, step); err != nil {
			e.logger.Error("fix step failed, rolling back",
				zap.Int("step", i),
				zap.Error(err),
			)
			rollbackErr := e.rollback(ctx, plan, i)
			return &ExecutionError{Cause: fmt.Errorf("修复步骤 %d 执行失败: %w", i+1, err), RolledBack: rollbackErr == nil}
		}
	}

	// 修复后验证
	for _, vstep := range plan.VerifySteps {
		if err := e.verify(ctx, vstep); err != nil {
			e.logger.Error("verification failed", zap.Error(err))
			rollbackErr := e.rollback(ctx, plan, len(plan.Steps))
			return &ExecutionError{Cause: fmt.Errorf("修复验证失败: %w", err), RolledBack: rollbackErr == nil}
		}
	}

	return nil
}

// generateFixPlan 根据诊断结果生成修复方案
func (e *Engine) generateFixPlan(diagnosis *models.Diagnosis) *FixPlan {
	// 根据根因类型生成对应的修复步骤
	plan := &FixPlan{
		IncidentID: diagnosis.IncidentID,
		Steps:      []FixStep{},
		Rollback:   []FixStep{},
	}

	// 这里可以根据 diagnosis.RootCause 智能匹配修复方案
	// 简化版：直接返回诊断建议中的修复步骤
	plan.Steps = append(plan.Steps, FixStep{
		Action:      diagnosis.FixType,
		Target:      diagnosis.IncidentID,
		Description: diagnosis.SuggestedFix,
	})
	plan.Rollback = append(plan.Rollback, FixStep{Action: "rollback", Target: diagnosis.IncidentID, Description: "恢复到修复前的模拟状态"})
	plan.VerifySteps = append(plan.VerifySteps, VerifyStep{CheckType: "scenario_state", Target: diagnosis.IncidentID, Condition: "recovered"})

	return plan
}

// SetTarget 将诊断中的事件 ID 替换为实际服务名，供 Mock 状态机执行与验证。
func (p *FixPlan) SetTarget(service string) {
	for index := range p.Steps {
		p.Steps[index].Target = service
	}
	for index := range p.Rollback {
		p.Rollback[index].Target = service
	}
	for index := range p.VerifySteps {
		p.VerifySteps[index].Target = service
	}
}

type ExecutionError struct {
	Cause      error
	RolledBack bool
}

func (e *ExecutionError) Error() string {
	if e.RolledBack {
		return e.Cause.Error() + "，已自动回滚"
	}
	return e.Cause.Error()
}

func (e *ExecutionError) Unwrap() error { return e.Cause }

func (e *Engine) executeStep(ctx context.Context, step FixStep) error {
	e.logger.Info("mock execute", zap.String("action", step.Action), zap.String("target", step.Target))
	if e.mockURL == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]string{"service": step.Target, "action": step.Action})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.mockURL+"/mock/actions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("模拟执行器返回 %s", resp.Status)
	}
	return nil
}

func (e *Engine) rollback(ctx context.Context, plan *FixPlan, failedStep int) error {
	e.logger.Info("rolling back", zap.String("incident", plan.IncidentID))
	for i := len(plan.Rollback) - 1; i >= 0; i-- {
		if err := e.executeStep(ctx, plan.Rollback[i]); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) verify(ctx context.Context, step VerifyStep) error {
	e.logger.Info("verifying", zap.String("check", step.CheckType), zap.String("target", step.Target))
	if e.mockURL == "" {
		return nil
	}
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("等待服务恢复超时")
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.mockURL+"/mock/scenarios/"+step.Target, nil)
			resp, err := e.httpCli.Do(req)
			if err != nil {
				continue
			}
			var payload struct {
				State string `json:"state"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
			resp.Body.Close()
			if decodeErr != nil || resp.StatusCode != http.StatusOK {
				continue
			}
			if payload.State == step.Condition {
				return nil
			}
			if payload.State == "failed" {
				return fmt.Errorf("修复后健康检查仍未通过")
			}
		}
	}
}
