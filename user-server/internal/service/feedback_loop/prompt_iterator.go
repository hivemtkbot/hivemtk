package feedbackloop

// prompt_iterator.go P0-5 Prompt 迭代器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十七章 §17.4.3
//
// 职责：基于低转化 SOP 节点 + 负反馈样本 → LLM 生成 Prompt 候选 → 入库 prompt_candidates
//
// 5 阶段流程：
//   1. 拉取节点当前 active Prompt
//   2. 拉取最近 7 天负反馈样本（reward < NegativeRewardThreshold）
//   3. LLM 生成 N 个改进版本
//   4. 入库 prompt_candidates（status=draft 或 approved，根据 AutoApprove）
//   5. 自动创建 A/B 测试（包含原 Prompt 作为兜底 arm_0）
//
// 关键设计：
//   - 样本数 < MinSamplesForIteration 拒绝迭代（防止噪声驱动）
//   - 候选永远保留原版作为兜底臂，防止新版本全面劣化
//   - 版本号递增规则：v1.0 → v1.1 → ... → v1.9 → v2.0

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
)

// PromptIterator Prompt 迭代器
type PromptIterator struct {
	db         *gorm.DB
	dispatcher LLMDispatcher
	config     PromptIteratorConfig
}

// NewPromptIterator 构造迭代器
func NewPromptIterator(db *gorm.DB, dispatcher LLMDispatcher, cfg PromptIteratorConfig) *PromptIterator {
	if cfg.MinSamplesForIteration == 0 {
		cfg.MinSamplesForIteration = 50
	}
	if cfg.NegativeRewardThreshold == 0 {
		cfg.NegativeRewardThreshold = -0.5
	}
	if cfg.CandidatesPerRun == 0 {
		cfg.CandidatesPerRun = 3
	}
	return &PromptIterator{
		db:         db,
		dispatcher: dispatcher,
		config:     cfg,
	}
}

// IterateForNode 为指定 SOP 节点迭代 Prompt
//
// 流程：
//  1. 拉取节点当前 active Prompt
//  2. 拉取最近 7 天负反馈样本
//  3. 样本数 < MinSamplesForIteration → 返回 ErrInsufficientSamples
//  4. LLM 生成 N 个候选
//  5. 入库 prompt_candidates（status=draft 或 approved）
//  6. AutoApprove=true 时自动创建 A/B 测试
//
// 返回：生成的候选列表
func (p *PromptIterator) IterateForNode(ctx context.Context, sopID uint, nodeID string) ([]model.PromptCandidate, error) {
	if p.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if sopID == 0 || nodeID == "" {
		return nil, ErrInvalidInput
	}

	// 1. 拉取当前 active Prompt
	var current model.PromptCandidate
	err := p.db.WithContext(ctx).
		Where("sop_id = ? AND sop_node_id = ? AND status = ?", sopID, nodeID, model.PromptCandidateStatusActive).
		First(&current).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrActivePromptNotFound
		}
		return nil, fmt.Errorf("query active prompt: %w", err)
	}

	// 2. 拉取负反馈样本
	//    先 COUNT 校验是否达到 MinSamplesForIteration 阈值（防止噪声驱动迭代）
	//    再 fetch 最多 20 条最低 reward 样本作为 LLM 上下文（避免 token 爆炸）
	since := time.Now().Add(-7 * 24 * time.Hour)
	totalNegative, err := p.countNegativeSamples(ctx, sopID, since)
	if err != nil {
		return nil, fmt.Errorf("count negative samples: %w", err)
	}
	if int(totalNegative) < p.config.MinSamplesForIteration {
		return nil, fmt.Errorf("%w: %d < %d", ErrInsufficientSamples, totalNegative, p.config.MinSamplesForIteration)
	}
	samples, err := p.fetchNegativeSamples(ctx, sopID, since)
	if err != nil {
		return nil, fmt.Errorf("fetch negative samples: %w", err)
	}

	// 3. LLM 生成候选
	candidates, err := p.generateCandidates(ctx, current, samples)
	if err != nil {
		return nil, fmt.Errorf("generate candidates: %w", err)
	}

	// 4. 入库
	now := time.Now()
	for i := range candidates {
		candidates[i].ParentID = current.ID
		candidates[i].SOPID = sopID
		candidates[i].SOPNodeID = nodeID
		candidates[i].Status = model.PromptCandidateStatusDraft
		candidates[i].Alpha = 2 // Beta(2,2) 弱先验
		candidates[i].Beta = 2
		candidates[i].GeneratedBy = "llm"
		if p.config.AutoApprove {
			candidates[i].Status = model.PromptCandidateStatusApproved
			candidates[i].ReviewedAt = &now
		}
		if err := p.db.WithContext(ctx).Create(&candidates[i]).Error; err != nil {
			// 单条失败不阻断，但记录到候选的 ImprovementNotes
			continue
		}
	}

	// 5. 自动创建 A/B 测试（包含原 Prompt 作为兜底臂）
	if p.config.AutoApprove && len(candidates) > 0 {
		p.createABTest(ctx, sopID, nodeID, current, candidates)
	}

	return candidates, nil
}

// fetchNegativeSamples 拉取负反馈样本
//
// 查询 feedback_events 中 reward < NegativeRewardThreshold 的记录
// 返回最多 20 条 reward 最低的样本（用于 LLM 上下文，避免 token 爆炸）
//
// 注意：本函数仅返回 Top-20 样本，不能用于阈值判定；
// 阈值判定应使用 countNegativeSamples 查询全量计数。
func (p *PromptIterator) fetchNegativeSamples(ctx context.Context, sopID uint, since time.Time) ([]negativeSample, error) {
	var rows []negativeSample
	err := p.db.WithContext(ctx).Raw(`
		SELECT fe.customer_msg, fe.ai_reply, fe.reward, fe.signal_key
		FROM feedback_events fe
		WHERE fe.sop_id = ? AND fe.created_at >= ? AND fe.reward < ?
		ORDER BY fe.reward ASC LIMIT 20`,
		sopID, since, p.config.NegativeRewardThreshold).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// countNegativeSamples 统计指定时间窗口内的负反馈样本总数
//
// 用于 MinSamplesForIteration 阈值判定（与 fetchNegativeSamples 的 LIMIT 20 解耦）
func (p *PromptIterator) countNegativeSamples(ctx context.Context, sopID uint, since time.Time) (int64, error) {
	var count int64
	err := p.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM feedback_events fe
		WHERE fe.sop_id = ? AND fe.created_at >= ? AND fe.reward < ?`,
		sopID, since, p.config.NegativeRewardThreshold).Scan(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// negativeSample 负反馈样本
type negativeSample struct {
	CustomerMsg string  `gorm:"column:customer_msg"`
	AIReply     string  `gorm:"column:ai_reply"`
	Reward      float64 `gorm:"column:reward"`
	SignalKey   string  `gorm:"column:signal_key"`
}

// generateCandidates LLM 生成候选 Prompt
//
// Prompt 模板：当前 System/User Prompt + N 条负反馈样本 → 输出 N 个改进版本 JSON
func (p *PromptIterator) generateCandidates(ctx context.Context, current model.PromptCandidate, samples []negativeSample) ([]model.PromptCandidate, error) {
	if p.dispatcher == nil {
		return nil, ErrDispatcherNotConfig
	}
	var sampleStrs []string
	for _, s := range samples {
		sampleStrs = append(sampleStrs, fmt.Sprintf("【客户】%s\n【AI 回复】%s\n【反馈】%s（reward=%.2f）",
			s.CustomerMsg, s.AIReply, s.SignalKey, s.Reward))
	}
	prompt := fmt.Sprintf(`你是 Prompt 优化工程师。以下是一个销售 SOP 节点的当前 Prompt 和 N 条负反馈样本：

【当前 System Prompt】
%s

【当前 User Prompt 模板】
%s

【负反馈样本】
%s

请生成 %d 个改进版本，每个版本说明改进点。输出 JSON 数组：
[
  {
    "title": "版本标题",
    "system_prompt": "改进后的 system prompt",
    "user_prompt_template": "改进后的 user prompt 模板",
    "improvement_notes": "改进点说明"
  }
]`,
		current.SystemPrompt, current.UserPromptTemplate,
		strings.Join(sampleStrs, "\n\n"), p.config.CandidatesPerRun)

	content, _, err := p.dispatcher.Dispatch(ctx, "high_quality", prompt, "你是 Prompt 优化工程师，严格输出 JSON 数组。", true, 2000)
	if err != nil {
		return nil, err
	}
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON content in LLM response: %s", content)
	}
	var raw []struct {
		Title              string `json:"title"`
		SystemPrompt       string `json:"system_prompt"`
		UserPromptTemplate string `json:"user_prompt_template"`
		ImprovementNotes   string `json:"improvement_notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("parse candidates: %w", err)
	}
	out := make([]model.PromptCandidate, 0, len(raw))
	for _, r := range raw {
		out = append(out, model.PromptCandidate{
			Scenario:           current.Scenario,
			Version:            nextVersion(current.Version),
			Title:              r.Title,
			SystemPrompt:       r.SystemPrompt,
			UserPromptTemplate: r.UserPromptTemplate,
			ImprovementNotes:   r.ImprovementNotes,
		})
	}
	return out, nil
}

// createABTest 自动创建 A/B 测试
//
// 实验 ID：sop{sopID}_node{nodeID}_{unix_time}
// arm_keys：["arm_0_original", "arm_1_v1.1", "arm_2_v1.2", ...]
// 原 Prompt 作为 arm_0（兜底臂，Beta(2,2) 弱先验）
// 新候选作为 arm_1..N
func (p *PromptIterator) createABTest(ctx context.Context, sopID uint, nodeID string, current model.PromptCandidate, newCandidates []model.PromptCandidate) {
	experimentID := fmt.Sprintf("sop%d_node%s_%d", sopID, nodeID, time.Now().Unix())
	armKeys := []string{"arm_0_original"}
	for i := range newCandidates {
		armKeys = append(armKeys, fmt.Sprintf("arm_%d_%s", i+1, newCandidates[i].Version))
	}
	now := time.Now()
	abTest := &model.PromptABTest{
		ExperimentID:   experimentID,
		ExperimentType: model.BanditExperimentTypePrompt,
		SOPID:          sopID,
		SOPNodeID:      nodeID,
		Name:           fmt.Sprintf("SOP %d 节点 %s Prompt 迭代", sopID, nodeID),
		ArmKeys:        model.JSONArray(armKeysToInterface(armKeys)),
		Config:         model.JSONMap{"min_traffic": 10, "max_traffic": 60, "min_samples": 100, "posterior_threshold": 0.95},
		Status:         model.PromptABTestStatusRunning,
		StartedAt:      &now,
		AutoPromote:    true,
	}
	if err := p.db.WithContext(ctx).Create(abTest).Error; err != nil {
		return
	}

	// 创建 bandit arms（原 Prompt 作为 arm_0）
	arms := []model.BanditArm{
		{
			ExperimentID:      experimentID,
			ExperimentType:    model.BanditExperimentTypePrompt,
			ArmKey:            "arm_0_original",
			SOPID:             sopID,
			PromptCandidateID: current.ID,
			Alpha:             2,
			Beta:              2,
			Status:            model.BanditArmStatusExploring,
		},
	}
	for i, c := range newCandidates {
		arms = append(arms, model.BanditArm{
			ExperimentID:      experimentID,
			ExperimentType:    model.BanditExperimentTypePrompt,
			ArmKey:            armKeys[i+1],
			SOPID:             sopID,
			PromptCandidateID: c.ID,
			Alpha:             2,
			Beta:              2,
			Status:            model.BanditArmStatusExploring,
		})
	}
	_ = p.db.WithContext(ctx).CreateInBatches(arms, 100).Error
}

// armKeysToInterface 将 []string 转为 []interface{}（用于 JSONArray）
func armKeysToInterface(keys []string) []any {
	out := make([]any, len(keys))
	for i, k := range keys {
		out[i] = k
	}
	return out
}

// nextVersion 版本号递增
//
// 规则：v1.0 → v1.1 → ... → v1.9 → v2.0
// 非法格式（如 "1.0" 无 v 前缀）则附加 .1
func nextVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "v1.1"
	}
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return v + ".1"
	}
	major := parts[0]
	minor := parts[1]
	var minorInt int
	if _, err := fmt.Sscanf(minor, "%d", &minorInt); err != nil {
		return v + ".1"
	}
	minorInt++
	if minorInt >= 10 {
		var majorInt int
		if strings.HasPrefix(major, "v") {
			if _, err := fmt.Sscanf(major[1:], "%d", &majorInt); err != nil {
				return v + ".1"
			}
		} else {
			if _, err := fmt.Sscanf(major, "%d", &majorInt); err != nil {
				return v + ".1"
			}
		}
		majorInt++
		return fmt.Sprintf("v%d.0", majorInt)
	}
	return fmt.Sprintf("%s.%d", major, minorInt)
}
