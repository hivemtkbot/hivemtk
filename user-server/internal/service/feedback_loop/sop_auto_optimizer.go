package feedbackloop

// sop_auto_optimizer.go P0-5 SOP 自动优化器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4.4
//
// 职责：消费 optimization_suggestions → 自动应用 → A/B 测试 → 自动选优/回滚
//
// 流程：
//   1. 扫描 pending suggestions
//   2. priority ≥ AutoApplyPriority 自动应用：
//        - prompt_rewrite   → 委托 PromptIterator（外部调用方）
//        - branch_prune     → 克隆 SOP + 标记节点 disabled + 创建 A/B 测试
//        - node_merge       → 合并相邻 action 节点
//        - add_objection    → 注入异议处理子分支
//        - add_empathy      → 修改 LLM 节点 system_prompt
//        - timing_adjust    → 调整 wait 节点 duration
//   3. 检查进行中的 A/B 测试是否需要回滚（转化率下降 / 投诉率上升）
//   4. 检查进行中的 A/B 测试是否可以收敛选优

import (
	"context"
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
)

// SOPAutoOptimizer SOP 自动优化器
type SOPAutoOptimizer struct {
	db     *gorm.DB
	bandit BanditAllocatorInterface
	config SOPAutoOptimizerConfig
}

// NewSOPAutoOptimizer 构造优化器
func NewSOPAutoOptimizer(db *gorm.DB, bandit BanditAllocatorInterface, cfg SOPAutoOptimizerConfig) *SOPAutoOptimizer {
	if cfg.AutoApplyPriority == 0 {
		cfg.AutoApplyPriority = 2
	}
	if cfg.RollbackDropThreshold == 0 {
		cfg.RollbackDropThreshold = 0.20
	}
	if cfg.RollbackComplaintRatio == 0 {
		cfg.RollbackComplaintRatio = 0.50
	}
	if cfg.ABTestDuration == 0 {
		cfg.ABTestDuration = 7 * 24 * time.Hour
	}
	return &SOPAutoOptimizer{
		db:     db,
		bandit: bandit,
		config: cfg,
	}
}

// ProcessPendingSuggestions 处理待审核建议
//
// 1. 高 priority 自动应用 + 创建 A/B 测试
// 2. 中低 priority 留待人工审核
// 3. 检查进行中的 A/B 测试是否需要回滚
// 4. 检查进行中的 A/B 测试是否可以收敛选优
func (o *SOPAutoOptimizer) ProcessPendingSuggestions(ctx context.Context) (*dto.OptimizationReport, error) {
	report := &dto.OptimizationReport{
		RunAt:  time.Now(),
		Errors: make([]string, 0),
	}
	if o.db == nil {
		return report, fmt.Errorf("db is nil")
	}

	// 1. 自动应用高 priority
	var autoApply []model.OptimizationSuggestion
	result := o.db.WithContext(ctx).
		Where("status = ? AND priority >= ?", model.SuggestionStatusPending, o.config.AutoApplyPriority).
		Find(&autoApply)
	if result.Error != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("query pending suggestions: %v", result.Error))
		return report, result.Error
	}
	report.PendingCount = len(autoApply)

	for i := range autoApply {
		if err := o.autoApply(ctx, &autoApply[i]); err != nil {
			report.FailedCount++
			report.Errors = append(report.Errors, fmt.Sprintf("apply suggestion %d: %v", autoApply[i].ID, err))
			continue
		}
		// 更新 suggestion 状态为 applied
		now := time.Now()
		o.db.WithContext(ctx).Model(&model.OptimizationSuggestion{}).
			Where("id = ?", autoApply[i].ID).
			Updates(map[string]any{
				"status":     model.SuggestionStatusApplied,
				"applied_at": now,
			})
		report.AppliedCount++
	}

	// 2. 检查进行中的 A/B 测试是否需要回滚
	o.checkAndRollback(ctx, report)

	// 3. 检查进行中的 A/B 测试是否可以收敛
	o.checkAndPromote(ctx, report)

	return report, nil
}

// autoApply 自动应用建议
//
// 根据 SuggestionType 分发到对应处理函数
func (o *SOPAutoOptimizer) autoApply(ctx context.Context, sug *model.OptimizationSuggestion) error {
	switch sug.SuggestionType {
	case model.SuggestionTypePromptRewrite:
		// 委托给 PromptIterator.IterateForNode（由调用方在外部触发）
		// 此处仅标记为已应用，实际 Prompt 候选生成由独立流程处理
		return nil
	case model.SuggestionTypeBranchPrune:
		return o.applyBranchPrune(ctx, sug)
	case model.SuggestionTypeNodeMerge:
		return o.applyNodeMerge(ctx, sug)
	case model.SuggestionTypeAddObjection:
		return o.applyAddObjection(ctx, sug)
	case model.SuggestionTypeAddEmpathy:
		return o.applyAddEmpathy(ctx, sug)
	case model.SuggestionTypeTimingAdjust:
		return o.applyTimingAdjust(ctx, sug)
	}
	return fmt.Errorf("unknown suggestion type: %s", sug.SuggestionType)
}

// applyBranchPrune 自动剪枝：克隆 SOP 为 variant B + 创建 A/B 测试
//
// 注意：实际节点 graph 修改逻辑由 SOPService 已有方法处理，此处仅创建 SOP 副本与 A/B 测试配置
func (o *SOPAutoOptimizer) applyBranchPrune(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original model.SOPAgent
		if err := tx.First(&original, sug.SOPID).Error; err != nil {
			return fmt.Errorf("query original sop: %w", err)
		}
		variantB := original
		variantB.ID = 0
		variantB.Name = original.Name + " [优化-剪枝]"
		if err := tx.Create(&variantB).Error; err != nil {
			return fmt.Errorf("create variant sop: %w", err)
		}
		return o.createSOPABTest(tx, original.ID, variantB.ID, "branch_prune")
	})
}

// applyNodeMerge 合并相邻 action 节点（占位实现，实际逻辑由 SOPService 处理）
func (o *SOPAutoOptimizer) applyNodeMerge(ctx context.Context, sug *model.OptimizationSuggestion) error {
	// 当前阶段仅创建 A/B 测试框架，节点合并的具体 graph 修改由运营人工审核后执行
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original model.SOPAgent
		if err := tx.First(&original, sug.SOPID).Error; err != nil {
			return err
		}
		variantB := original
		variantB.ID = 0
		variantB.Name = original.Name + " [优化-节点合并]"
		if err := tx.Create(&variantB).Error; err != nil {
			return err
		}
		return o.createSOPABTest(tx, original.ID, variantB.ID, "node_merge")
	})
}

// applyAddObjection 注入异议处理子分支（占位实现）
func (o *SOPAutoOptimizer) applyAddObjection(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original model.SOPAgent
		if err := tx.First(&original, sug.SOPID).Error; err != nil {
			return err
		}
		variantB := original
		variantB.ID = 0
		variantB.Name = original.Name + " [优化-异议处理]"
		if err := tx.Create(&variantB).Error; err != nil {
			return err
		}
		return o.createSOPABTest(tx, original.ID, variantB.ID, "add_objection")
	})
}

// applyAddEmpathy 修改 LLM 节点 system_prompt 补充共情（占位实现）
func (o *SOPAutoOptimizer) applyAddEmpathy(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original model.SOPAgent
		if err := tx.First(&original, sug.SOPID).Error; err != nil {
			return err
		}
		variantB := original
		variantB.ID = 0
		variantB.Name = original.Name + " [优化-共情补充]"
		if err := tx.Create(&variantB).Error; err != nil {
			return err
		}
		return o.createSOPABTest(tx, original.ID, variantB.ID, "add_empathy")
	})
}

// applyTimingAdjust 调整 wait 节点 duration（占位实现）
func (o *SOPAutoOptimizer) applyTimingAdjust(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original model.SOPAgent
		if err := tx.First(&original, sug.SOPID).Error; err != nil {
			return err
		}
		variantB := original
		variantB.ID = 0
		variantB.Name = original.Name + " [优化-时机调整]"
		if err := tx.Create(&variantB).Error; err != nil {
			return err
		}
		return o.createSOPABTest(tx, original.ID, variantB.ID, "timing_adjust")
	})
}

// createSOPABTest 创建 SOP variant A/B 测试
//
// experiment_id: sop_variant_{originalID}_{variantID}_{unix}
// arm_keys: ["arm_a_original", "arm_b_variant"]
// 原 SOP 作为 arm_a（兜底），优化后 SOP 作为 arm_b
func (o *SOPAutoOptimizer) createSOPABTest(tx *gorm.DB, originalID, variantID uint, optimizationType string) error {
	experimentID := fmt.Sprintf("sop_variant_%d_%d_%d", originalID, variantID, time.Now().Unix())
	now := time.Now()
	armKeys := []any{"arm_a_original", "arm_b_variant"}
	abTest := &model.PromptABTest{
		ExperimentID:   experimentID,
		ExperimentType: model.BanditExperimentTypeSOPVariant,
		SOPID:          originalID,
		Name:           fmt.Sprintf("SOP %d 优化（%s）→ variant %d", originalID, optimizationType, variantID),
		Description:    fmt.Sprintf("优化类型: %s, 原 SOP ID: %d, 新 variant ID: %d", optimizationType, originalID, variantID),
		ArmKeys:        model.JSONArray(armKeys),
		Config:         model.JSONMap{"min_traffic": 10, "max_traffic": 60, "min_samples": 100, "posterior_threshold": 0.95},
		Status:         model.PromptABTestStatusRunning,
		StartedAt:      &now,
		AutoPromote:    true,
	}
	if err := tx.Create(abTest).Error; err != nil {
		return fmt.Errorf("create ab test: %w", err)
	}

	// 创建 bandit arms
	arms := []model.BanditArm{
		{
			ExperimentID:   experimentID,
			ExperimentType: model.BanditExperimentTypeSOPVariant,
			ArmKey:         "arm_a_original",
			SOPID:          originalID,
			Variant:        "A",
			Alpha:          2,
			Beta:           2,
			Status:         model.BanditArmStatusExploring,
		},
		{
			ExperimentID:   experimentID,
			ExperimentType: model.BanditExperimentTypeSOPVariant,
			ArmKey:         "arm_b_variant",
			SOPID:          variantID,
			Variant:        "B",
			Alpha:          2,
			Beta:           2,
			Status:         model.BanditArmStatusExploring,
		},
	}
	if err := tx.CreateInBatches(arms, 100).Error; err != nil {
		return fmt.Errorf("create bandit arms: %w", err)
	}
	return nil
}

// checkAndRollback 检查 A/B 测试是否需要回滚
//
// 回滚条件：
//  1. 实验组 vs 对照组转化率下降 > RollbackDropThreshold（20%）
//  2. 实验组 vs 对照组投诉率上升 > RollbackComplaintRatio（50%）
func (o *SOPAutoOptimizer) checkAndRollback(ctx context.Context, report *dto.OptimizationReport) {
	var tests []model.PromptABTest
	o.db.WithContext(ctx).
		Where("status = ? AND experiment_type = ?", model.PromptABTestStatusRunning, model.BanditExperimentTypeSOPVariant).
		Find(&tests)
	for _, t := range tests {
		// 比较实验组 vs 对照组的转化率
		controlRate, experimentRate := o.fetchConversionRates(ctx, t.ID)
		if controlRate > 0 && (controlRate-experimentRate)/controlRate > o.config.RollbackDropThreshold {
			if err := o.rollbackTest(ctx, t.ID, "conversion_drop"); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("rollback test %d (conversion_drop): %v", t.ID, err))
				continue
			}
			report.RolledBackCount++
			continue // 已回滚则不再检查投诉率
		}
		// 投诉率检查
		controlComplaint, expComplaint := o.fetchComplaintRates(ctx, t.ID)
		if controlComplaint > 0 && (expComplaint-controlComplaint)/controlComplaint > o.config.RollbackComplaintRatio {
			if err := o.rollbackTest(ctx, t.ID, "complaint_spike"); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("rollback test %d (complaint_spike): %v", t.ID, err))
				continue
			}
			report.RolledBackCount++
		}
	}
}

// checkAndPromote 检查 A/B 测试是否可以收敛选优
func (o *SOPAutoOptimizer) checkAndPromote(ctx context.Context, report *dto.OptimizationReport) {
	if o.bandit == nil {
		return
	}
	var tests []model.PromptABTest
	o.db.WithContext(ctx).Where("status = ?", model.PromptABTestStatusRunning).Find(&tests)
	for _, t := range tests {
		winner, ok := o.bandit.CheckConvergence(ctx, t.ExperimentID)
		if !ok {
			continue
		}
		if err := o.bandit.PromoteArm(ctx, t.ExperimentID, winner); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("promote arm (test=%d): %v", t.ID, err))
			continue
		}
		// 更新测试状态
		now := time.Now()
		o.db.WithContext(ctx).Model(&model.PromptABTest{}).
			Where("id = ?", t.ID).
			Updates(map[string]any{
				"status":         model.PromptABTestStatusCompleted,
				"winner_arm_key": winner,
				"ended_at":       now,
			})
		report.PromotedCount++
	}
}

// fetchConversionRates 拉取对照组/实验组的转化率
//
// 基于 feedback_signals 表：
//   - 对照组：variant=A, outcome=success 的占比
//   - 实验组：variant=B, outcome=success 的占比
func (o *SOPAutoOptimizer) fetchConversionRates(ctx context.Context, testID uint) (controlRate, experimentRate float64) {
	var test model.PromptABTest
	if err := o.db.WithContext(ctx).First(&test, testID).Error; err != nil {
		return 0, 0
	}
	// 对照组转化率
	var controlTotal, controlSuccess int64
	o.db.WithContext(ctx).Model(&model.FeedbackSignal{}).
		Where("sop_id = ? AND variant = ?", test.SOPID, "A").
		Count(&controlTotal)
	o.db.WithContext(ctx).Model(&model.FeedbackSignal{}).
		Where("sop_id = ? AND variant = ? AND outcome = ?", test.SOPID, "A", model.FeedbackSignalOutcomeSuccess).
		Count(&controlSuccess)
	if controlTotal > 0 {
		controlRate = float64(controlSuccess) / float64(controlTotal)
	}
	// 实验组转化率
	var expTotal, expSuccess int64
	o.db.WithContext(ctx).Model(&model.FeedbackSignal{}).
		Where("sop_id = ? AND variant = ?", test.SOPID, "B").
		Count(&expTotal)
	o.db.WithContext(ctx).Model(&model.FeedbackSignal{}).
		Where("sop_id = ? AND variant = ? AND outcome = ?", test.SOPID, "B", model.FeedbackSignalOutcomeSuccess).
		Count(&expSuccess)
	if expTotal > 0 {
		experimentRate = float64(expSuccess) / float64(expTotal)
	}
	return controlRate, experimentRate
}

// fetchComplaintRates 拉取对照组/实验组的投诉率
//
// 基于 feedback_events 表：
//   - 投诉率 = signal_key='complaint' 事件数 / 总事件数
func (o *SOPAutoOptimizer) fetchComplaintRates(ctx context.Context, testID uint) (controlRate, experimentRate float64) {
	var test model.PromptABTest
	if err := o.db.WithContext(ctx).First(&test, testID).Error; err != nil {
		return 0, 0
	}
	// 对照组投诉率
	var controlTotal, controlComplaint int64
	o.db.WithContext(ctx).Model(&model.FeedbackEvent{}).
		Where("sop_id = ? AND variant = ?", test.SOPID, "A").
		Count(&controlTotal)
	o.db.WithContext(ctx).Model(&model.FeedbackEvent{}).
		Where("sop_id = ? AND variant = ? AND signal_key = ?", test.SOPID, "A", model.FeedbackSignalComplaint).
		Count(&controlComplaint)
	if controlTotal > 0 {
		controlRate = float64(controlComplaint) / float64(controlTotal)
	}
	// 实验组投诉率
	var expTotal, expComplaint int64
	o.db.WithContext(ctx).Model(&model.FeedbackEvent{}).
		Where("sop_id = ? AND variant = ?", test.SOPID, "B").
		Count(&expTotal)
	o.db.WithContext(ctx).Model(&model.FeedbackEvent{}).
		Where("sop_id = ? AND variant = ? AND signal_key = ?", test.SOPID, "B", model.FeedbackSignalComplaint).
		Count(&expComplaint)
	if expTotal > 0 {
		experimentRate = float64(expComplaint) / float64(expTotal)
	}
	return controlRate, experimentRate
}

// rollbackTest 回滚 A/B 测试
//
// 1. test.status = rolled_back
// 2. 所有 bandit_arms status = retired, retired_at = NOW()
// 3. 记录 retired_reason
func (o *SOPAutoOptimizer) rollbackTest(ctx context.Context, testID uint, reason string) error {
	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var test model.PromptABTest
		if err := tx.First(&test, testID).Error; err != nil {
			return err
		}
		now := time.Now()
		// 更新测试状态
		if err := tx.Model(&model.PromptABTest{}).
			Where("id = ?", testID).
			Updates(map[string]any{
				"status":   model.PromptABTestStatusRolledBack,
				"ended_at": now,
			}).Error; err != nil {
			return err
		}
		// 退役所有 bandit arms
		return tx.Model(&model.BanditArm{}).
			Where("experiment_id = ?", test.ExperimentID).
			Updates(map[string]any{
				"status":     model.BanditArmStatusRetired,
				"retired_at": now,
				"updated_at": now,
			}).Error
	})
}
