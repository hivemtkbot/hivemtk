package service

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
)

// sop_abtest.go SOP A/B 测试流量分配与统计（PRD §5.2 P0-2 G2 缺口修复）
//
// 设计目标：
//  1. 支持基于客户 ID 一致性哈希的稳定分流（同一客户始终命中同一 variant）
//  2. 支持 variant 权重分配（如 A:50%, B:50% 或 A:70%, B:20%, C:10%）
//  3. 支持 variant 维度的执行/成功统计，用于效果对比
//  4. 不影响未启用 A/B 测试的 SOP（向后兼容）
//
// 注：SOPABTestConfig / SOPABTestVariant 类型及 Validate / SelectVariant 方法
// 已迁移至 dto 包。service 包通过类型别名（type alias）保持向后兼容。
// 因 Go 不允许为非本地类型（alias）定义方法，方法定义必须放在 dto 包内。

// SOPABTestVariant A/B 测试 variant 定义
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type SOPABTestVariant = dto.SOPABTestVariant

// SOPABTestConfig A/B 测试配置
// 已迁移至 dto 包
type SOPABTestConfig = dto.SOPABTestConfig

// ParseSOPABTestConfig 从 model.SOPAgent.ABTestConfig (JSONMap) 解析配置
// 配置为空或解析失败时返回 disabled 配置
func ParseSOPABTestConfig(raw model.JSONMap) SOPABTestConfig {
	cfg := SOPABTestConfig{Enabled: false}
	if raw == nil {
		return cfg
	}

	// 通过 JSON 序列化/反序列化转换
	data, err := json.Marshal(raw)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// SOPABTestVariantStats A/B 测试 variant 统计
type SOPABTestVariantStats struct {
	Variant		string	`json:"variant"`
	ExecutionCount	int64	`json:"execution_count"`
	SuccessCount	int64	`json:"success_count"`
	FailedCount	int64	`json:"failed_count"`
	RunningCount	int64	`json:"running_count"`
	SuccessRate	float64	`json:"success_rate"`	// 成功率（百分比）
}

// GetABTestStats 查询指定 SOP 的 A/B 测试 variant 统计
// 未启用 A/B 测试时返回空切片
func (s *SOPService) GetABTestStats(ctx context.Context, sopID uint) ([]SOPABTestVariantStats, error) {
	agent, err := s.Get(ctx, sopID)
	if err != nil {
		return nil, err
	}

	cfg := ParseSOPABTestConfig(agent.ABTestConfig)
	if !cfg.Enabled {
		return []SOPABTestVariantStats{}, nil
	}

	stats := make([]SOPABTestVariantStats, 0, len(cfg.Variants))
	for _, v := range cfg.Variants {
		var execCount, successCount, failedCount, runningCount int64

		// 总执行数
		if err := s.db.Model(&model.SOPExecution{}).
			Where("sop_id = ? AND variant = ?", sopID, v.Name).
			Count(&execCount).Error; err != nil {
			return nil, fmt.Errorf("查询 variant [%s] 执行数失败：%w", v.Name, err)
		}
		// 成功数
		if err := s.db.Model(&model.SOPExecution{}).
			Where("sop_id = ? AND variant = ? AND status = ?", sopID, v.Name, SOPStatusSuccess).
			Count(&successCount).Error; err != nil {
			return nil, fmt.Errorf("查询 variant [%s] 成功数失败：%w", v.Name, err)
		}
		// 失败数
		if err := s.db.Model(&model.SOPExecution{}).
			Where("sop_id = ? AND variant = ? AND status = ?", sopID, v.Name, SOPStatusFailed).
			Count(&failedCount).Error; err != nil {
			return nil, fmt.Errorf("查询 variant [%s] 失败数失败：%w", v.Name, err)
		}
		// 运行中
		if err := s.db.Model(&model.SOPExecution{}).
			Where("sop_id = ? AND variant = ? AND status = ?", sopID, v.Name, SOPStatusRunning).
			Count(&runningCount).Error; err != nil {
			return nil, fmt.Errorf("查询 variant [%s] 运行中数失败：%w", v.Name, err)
		}

		successRate := 0.0
		if execCount > 0 {
			successRate = float64(successCount) / float64(execCount) * 100
		}

		stats = append(stats, SOPABTestVariantStats{
			Variant:	v.Name,
			ExecutionCount:	execCount,
			SuccessCount:	successCount,
			FailedCount:	failedCount,
			RunningCount:	runningCount,
			SuccessRate:	successRate,
		})
	}

	return stats, nil
}

// UpdateABTestConfig 更新 SOP 的 A/B 测试配置
func (s *SOPService) UpdateABTestConfig(ctx context.Context, sopID uint, cfg SOPABTestConfig) (*model.SOPAgent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("A/B 测试配置非法：%w", err)
	}

	agent, err := s.Get(ctx, sopID)
	if err != nil {
		return nil, err
	}

	// 序列化为 JSONMap
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("A/B 测试配置序列化失败：%w", err)
	}
	abMap := model.JSONMap{}
	if err := json.Unmarshal(data, &abMap); err != nil {
		return nil, fmt.Errorf("A/B 测试配置转换失败：%w", err)
	}

	agent.ABTestConfig = abMap
	agent.Version++

	if err := s.db.Save(agent).Error; err != nil {
		return nil, err
	}
	return agent, nil
}

// resolveABTestVariant 在 Execute 时解析 variant
// 返回 variant name 和对应的 SOP 图 ID（0 表示用主图）
// 未启用 A/B 测试时返回 ("", 0)
func (s *SOPService) resolveABTestVariant(ctx context.Context, agent *model.SOPAgent, customerID string) (variantName string, sopGraphID uint, err error) {
	cfg := ParseSOPABTestConfig(agent.ABTestConfig)
	if !cfg.Enabled {
		return "", 0, nil
	}
	if err := cfg.Validate(); err != nil {
		return "", 0, err
	}
	variant := cfg.SelectVariant(customerID)
	return variant.Name, variant.SOPGraphID, nil
}

// loadSOPGraphByID 根据 SOP ID 加载 SOPGraph（用于 A/B 测试 variant 切换图）
// graphID == 0 表示用 agent.SOPGraph 主图
func (s *SOPService) loadSOPGraph(ctx context.Context, agent *model.SOPAgent, graphID uint) (SOPGraph, error) {
	if graphID == 0 {
		var graph SOPGraph
		if err := json.Unmarshal(mustJSON(agent.SOPGraph), &graph); err != nil {
			return SOPGraph{}, ErrSOPInvalidGraph
		}
		return graph, nil
	}

	// 加载指定 ID 的 SOP 图
	var target model.SOPAgent
	if err := s.db.First(ctx, &target, graphID).Error; err != nil {
		return SOPGraph{}, fmt.Errorf("variant SOP 图加载失败（sop_id=%d）：%w", graphID, err)
	}
	var graph SOPGraph
	if err := json.Unmarshal(mustJSON(target.SOPGraph), &graph); err != nil {
		return SOPGraph{}, ErrSOPInvalidGraph
	}
	return graph, nil
}

// 用 gorm.Expr 占位避免 unused 警告（保留扩展点）
var _ = gorm.Expr
