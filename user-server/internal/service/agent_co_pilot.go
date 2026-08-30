// Package service - Agent Co-Pilot 自动执行（G1）
//
// 把现有审批门（approval gate）从"人工审核"扩展为"可配置阈值自动放行"。
// 当 AI 生成内容满足质量阈值时自动执行，否则进入审批队列。
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"hivemtk-user/internal/repository"
)

// CoPilotAutoExecuteConfig 自动执行配置
//
// 存储于 system_config_kv，key = "co_pilot_auto_execute"
type CoPilotAutoExecuteConfig struct {
	Enabled            bool    `json:"enabled"`
	ConfidenceThreshold float64 `json:"confidence_threshold"` // 置信度阈值，默认 0.85
	CostLimit          float64 `json:"cost_limit"`           // 单次成本上限（USD），默认 0.5
}

// 默认配置
func defaultCoPilotConfig() CoPilotAutoExecuteConfig {
	return CoPilotAutoExecuteConfig{
		Enabled:            false,
		ConfidenceThreshold: 0.85,
		CostLimit:          0.5,
	}
}

// AgentCoPilotService Agent Co-Pilot 自动执行服务
type AgentCoPilotService struct {
	configKV repository.SystemConfigKVRepository
}

// NewAgentCoPilotService 创建服务实例
func NewAgentCoPilotService() *AgentCoPilotService {
	return &AgentCoPilotService{
		configKV: repository.NewSystemConfigKVRepository(),
	}
}

// GetConfig 获取当前自动执行配置
func (s *AgentCoPilotService) GetConfig(ctx context.Context) (*CoPilotAutoExecuteConfig, error) {
	raw, err := s.configKV.Get(ctx, "co_pilot_auto_execute")
	if err != nil {
		return nil, fmt.Errorf("COPILOT_CONFIG_001: 读取配置失败: %w", err)
	}
	if raw == "" {
		cfg := defaultCoPilotConfig()
		return &cfg, nil
	}
	var cfg CoPilotAutoExecuteConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("COPILOT_CONFIG_002: 配置解析失败: %w", err)
	}
	return &cfg, nil
}

// SaveConfig 保存自动执行配置
func (s *AgentCoPilotService) SaveConfig(ctx context.Context, cfg *CoPilotAutoExecuteConfig) error {
	if cfg == nil {
		return fmt.Errorf("COPILOT_CONFIG_003: 配置不能为空")
	}
	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return fmt.Errorf("COPILOT_CONFIG_004: confidence_threshold 必须在 [0,1] 范围内")
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("COPILOT_CONFIG_005: 配置序列化失败: %w", err)
	}
	_, err = s.configKV.Upsert(ctx, "co_pilot_auto_execute", string(data))
	if err != nil {
		return fmt.Errorf("COPILOT_CONFIG_006: 配置保存失败: %w", err)
	}
	return nil
}

// AutoExecuteDecision 自动执行决策结果
type AutoExecuteDecision struct {
	AutoApproved bool    `json:"auto_approved"`
	Reason       string  `json:"reason"`
	Confidence   float64 `json:"confidence"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// Evaluate 评估 AI 生成内容是否满足自动执行条件。
//
// 判定逻辑（三层门禁）：
//  1. 功能总开关 enabled=false → 必须人工审批
//  2. confidence < threshold → 必须人工审批（AI 不够自信）
//  3. estimated_cost > cost_limit → 必须人工审批（成本超限）
//
// 全部满足则 auto_approved=true，直接跳过审批门执行。
func (s *AgentCoPilotService) Evaluate(ctx context.Context, confidence float64, estimatedCost float64) (*AutoExecuteDecision, error) {
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("COPILOT_EVAL_001: 获取配置失败: %w", err)
	}

	decision := &AutoExecuteDecision{
		Confidence:    confidence,
		EstimatedCost: estimatedCost,
	}

	if !cfg.Enabled {
		decision.AutoApproved = false
		decision.Reason = "co_pilot_auto_execute 功能未启用"
		return decision, nil
	}

	if confidence < cfg.ConfidenceThreshold {
		decision.AutoApproved = false
		decision.Reason = fmt.Sprintf("置信度 %.2f < 阈值 %.2f", confidence, cfg.ConfidenceThreshold)
		return decision, nil
	}

	if estimatedCost > cfg.CostLimit {
		decision.AutoApproved = false
		decision.Reason = fmt.Sprintf("预估成本 %.4f > 上限 %.4f", estimatedCost, cfg.CostLimit)
		return decision, nil
	}

	decision.AutoApproved = true
	decision.Reason = fmt.Sprintf("置信度 %.2f >= 阈值 %.2f 且成本 %.4f <= 上限 %.4f",
		confidence, cfg.ConfidenceThreshold, estimatedCost, cfg.CostLimit)
	return decision, nil
}
