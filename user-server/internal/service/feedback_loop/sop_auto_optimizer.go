package feedbackloop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// SOPAutoOptimizer SOP 自动优化器
type SOPAutoOptimizer struct {
	db      *gorm.DB
	repo    *repository.FeedbackLoopRepository
	bandit  BanditAllocatorInterface
	gateLLM gateLLM // L-1 验证门：nil 时黄金回归门 fail-closed（不自动应用）
	config  SOPAutoOptimizerConfig
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
		repo:   repository.NewFeedbackLoopRepositoryWithDB(db),
		bandit: bandit,
		config: cfg,
	}
}

// getRepo 获取 repository
//
// 兼容测试中直接构造 &SOPAutoOptimizer{db: db} 的场景：此时 repo 为 nil，
// 通过 db 懒加载 repository。生产路径在构造函数中已设置 repo。
func (o *SOPAutoOptimizer) getRepo() *repository.FeedbackLoopRepository {
	if o.repo == nil && o.db != nil {
		o.repo = repository.NewFeedbackLoopRepositoryWithDB(o.db)
	}
	return o.repo
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
	repo := o.getRepo()
	if repo == nil {
		return report, fmt.Errorf("repo is nil")
	}

	autoApply, err := repo.ListPendingSuggestions(ctx, o.config.AutoApplyPriority)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("query pending suggestions: %v", err))
		return report, err
	}
	report.PendingCount = len(autoApply)

	for i := range autoApply {
		// L-1 验证门：候选须过结构/合规/黄金回归门后才允许自动应用（dry_run→gate→apply）。
		// 近期已检查且未过的建议跳过，防止 cron 每轮重复烧 LLM。
		if o.recentlyGateChecked(&autoApply[i]) {
			continue
		}
		gateRes := o.passGate(ctx, &autoApply[i])
		o.recordGateResult(ctx, autoApply[i].ID, gateRes)
		if !gateRes.Passed {
			logger.Warnf("[sop_gate] 建议 %d 未过验证门，转人工审核: %s", autoApply[i].ID, gateRes.Reason)
			continue
		}
		if err := o.autoApply(ctx, &autoApply[i]); err != nil {
			report.FailedCount++
			report.Errors = append(report.Errors, fmt.Sprintf("apply suggestion %d: %v", autoApply[i].ID, err))
			continue
		}
		if err := repo.MarkSuggestionApplied(ctx, autoApply[i].ID, time.Now()); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("mark suggestion %d applied: %v", autoApply[i].ID, err))
		}
		report.AppliedCount++
	}

	o.checkAndRollback(ctx, report)

	o.checkAndPromote(ctx, report)

	return report, nil
}

// autoApply 自动应用建议
//
// 根据 SuggestionType 分发到对应处理函数
func (o *SOPAutoOptimizer) autoApply(ctx context.Context, sug *model.OptimizationSuggestion) error {
	switch sug.SuggestionType {
	case model.SuggestionTypePromptRewrite:
		return o.applyPromptRewrite(ctx, sug)
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

// applyBranchPrune 自动剪枝：克隆 SOP 为 variant B（真实删节点）+ 创建 A/B 测试
func (o *SOPAutoOptimizer) applyBranchPrune(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-剪枝]", "branch_prune", SOPGraphMutatorForExperiment("branch_prune"))
}

// applyNodeMerge 合并相邻 message 节点（创建 SOP 变体 + AB 测试，真实落地）
func (o *SOPAutoOptimizer) applyNodeMerge(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-节点合并]", "node_merge", SOPGraphMutatorForExperiment("node_merge"))
}

// applyAddObjection 注入异议处理子分支（创建 SOP 变体 + AB 测试，真实落地）
func (o *SOPAutoOptimizer) applyAddObjection(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-异议处理]", "add_objection", SOPGraphMutatorForExperiment("add_objection"))
}

// applyAddEmpathy 修改 LLM/message 节点 prompt 补充共情（创建 SOP 变体 + AB 测试，真实落地）
func (o *SOPAutoOptimizer) applyAddEmpathy(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-共情补充]", "add_empathy", SOPGraphMutatorForExperiment("add_empathy"))
}

// applyTimingAdjust 调整 wait 节点 duration（创建 SOP 变体 + AB 测试，真实落地）
func (o *SOPAutoOptimizer) applyTimingAdjust(ctx context.Context, sug *model.OptimizationSuggestion) error {
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-时机调整]", "timing_adjust", SOPGraphMutatorForExperiment("timing_adjust"))
}

// applyPromptRewrite R55 T1：LLM 节点转化率低 → 用 LLM 重写该节点 prompt，克隆为 variant B
//
// 此前该类型建议直接 return nil（静默吞掉），建议应用数虚低且闭环缺失。
// 重写失败（LLM 不可用/节点不存在）返回错误，由上层转人工审核，不静默。
func (o *SOPAutoOptimizer) applyPromptRewrite(ctx context.Context, sug *model.OptimizationSuggestion) error {
	if o.gateLLM == nil {
		return fmt.Errorf("prompt_rewrite 需要 LLM 能力，当前未注入 dispatcher")
	}
	newPrompt, err := o.rewriteNodePrompt(ctx, sug)
	if err != nil {
		return fmt.Errorf("rewrite prompt: %w", err)
	}
	mutator := SOPGraphMutatorForNodePrompt(sug.NodeID, newPrompt)
	return o.getRepo().CloneSOPAndCreateABTestMutated(ctx, sug.SOPID, " [优化-Prompt重写]", "prompt_rewrite", mutator)
}

// rewriteNodePrompt 用 LLM 依据建议文本重写节点 prompt
func (o *SOPAutoOptimizer) rewriteNodePrompt(ctx context.Context, sug *model.OptimizationSuggestion) (string, error) {
	systemPrompt := `你是销售 SOP 优化专家。给定一个 LLM 节点的当前 prompt 和优化建议，输出改进后的完整 prompt。
要求：保留原有业务意图与合规边界；按建议落实改进；输出仅含新 prompt 正文，不要解释。`
	userPrompt := fmt.Sprintf("当前 prompt：\n%s\n\n优化建议：\n%s\n\n输出改进后的 prompt：", sug.SuggestionText, sug.SuggestionText)
	res, err := o.gateLLM.Dispatch(ctx, llm.DispatchRequest{
		Scenario:     llm.ScenarioHighQuality,
		SystemPrompt: systemPrompt,
		Prompt:       userPrompt,
		MaxTokens:    1024,
		Temperature:  0.3,
	})
	if err != nil {
		return "", err
	}
	out := ""
	if res != nil {
		out = res.Content
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", fmt.Errorf("LLM 返回空 prompt")
	}
	return out, nil
}

// checkAndRollback 检查 A/B 测试是否需要回滚
//
// 回滚条件：
//  1. 实验组 vs 对照组转化率下降 > RollbackDropThreshold（20%）
//  2. 实验组 vs 对照组投诉率上升 > RollbackComplaintRatio（50%）
func (o *SOPAutoOptimizer) checkAndRollback(ctx context.Context, report *dto.OptimizationReport) {
	tests, err := o.getRepo().ListRunningABTestsByType(ctx, model.BanditExperimentTypeSOPVariant)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("list running ab tests: %v", err))
		return
	}
	for _, t := range tests {
		controlRate, experimentRate := o.fetchConversionRates(ctx, t.ID)
		if controlRate > 0 && (controlRate-experimentRate)/controlRate > o.config.RollbackDropThreshold {
			if err := o.rollbackTest(ctx, t.ID, "conversion_drop"); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("rollback test %d (conversion_drop): %v", t.ID, err))
				continue
			}
			report.RolledBackCount++
			continue
		}
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
	tests, err := o.getRepo().ListRunningABTests(ctx)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("list running ab tests for promote: %v", err))
		return
	}
	for _, t := range tests {
		winner, ok := o.bandit.CheckConvergence(ctx, t.ExperimentID)
		if !ok {
			continue
		}
		if err := o.bandit.PromoteArm(ctx, t.ExperimentID, winner); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("promote arm (test=%d): %v", t.ID, err))
			continue
		}
		now := time.Now()
		if err := o.getRepo().UpdateABTestFields(ctx, t.ID, map[string]any{
			"status":         model.PromptABTestStatusCompleted,
			"winner_arm_key": winner,
			"ended_at":       now,
		}); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("update ab test %d to completed: %v", t.ID, err))
		}
		report.PromotedCount++
	}
}

// fetchConversionRates 拉取对照组/实验组的转化率
//
// 基于 feedback_signals 表：
//   - 对照组：variant=A, outcome=success 的占比
//   - 实验组：variant=B, outcome=success 的占比
func (o *SOPAutoOptimizer) fetchConversionRates(ctx context.Context, testID uint) (controlRate, experimentRate float64) {
	repo := o.getRepo()
	test, err := repo.GetPromptABTest(ctx, testID)
	if err != nil || test == nil {
		return 0, 0
	}
	controlTotal, _ := repo.CountFeedbackSignalsByVariant(ctx, test.SOPID, "A")
	controlSuccess, _ := repo.CountFeedbackSignalsByVariantAndOutcome(ctx, test.SOPID, "A", model.FeedbackSignalOutcomeSuccess)
	if controlTotal > 0 {
		controlRate = float64(controlSuccess) / float64(controlTotal)
	}
	expTotal, _ := repo.CountFeedbackSignalsByVariant(ctx, test.SOPID, "B")
	expSuccess, _ := repo.CountFeedbackSignalsByVariantAndOutcome(ctx, test.SOPID, "B", model.FeedbackSignalOutcomeSuccess)
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
	repo := o.getRepo()
	test, err := repo.GetPromptABTest(ctx, testID)
	if err != nil || test == nil {
		return 0, 0
	}
	controlTotal, _ := repo.CountFeedbackEventsByVariant(ctx, test.SOPID, "A")
	controlComplaint, _ := repo.CountFeedbackEventsByVariantAndSignalKey(ctx, test.SOPID, "A", model.FeedbackSignalComplaint)
	if controlTotal > 0 {
		controlRate = float64(controlComplaint) / float64(controlTotal)
	}
	expTotal, _ := repo.CountFeedbackEventsByVariant(ctx, test.SOPID, "B")
	expComplaint, _ := repo.CountFeedbackEventsByVariantAndSignalKey(ctx, test.SOPID, "B", model.FeedbackSignalComplaint)
	if expTotal > 0 {
		experimentRate = float64(expComplaint) / float64(expTotal)
	}
	return controlRate, experimentRate
}

// rollbackTest 回滚 A/B 测试
//
// 1. test.status = rolled_back
// 2. 所有 bandit_arms status = retired, retired_at = NOW()
// 3. 记录 retired_reason（reason 参数保留用于日志/未来扩展，当前未持久化）
func (o *SOPAutoOptimizer) rollbackTest(ctx context.Context, testID uint, reason string) error {
	return o.getRepo().RollbackABTest(ctx, testID)
}
