package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ============================================================================
// P1-3 G7 反馈学习闭环服务
// ----------------------------------------------------------------------------
// 对应 PRD §5.2 P1-3 G7：系统自我进化
//
// 三大能力：
//  1. 销冠画像 5 维度提取（从对话记录自动提取能力维度）
//  2. SOP 节点转化率分析（统计每个节点的进入→推进→流失）
//  3. 低转化节点优化建议生成（自动生成 prompt 重写 / 分支剪枝等建议）
//
// 设计原则：
//   - 不依赖 LLM（规则 + 关键词提取，便于测试和降级）
//   - 持久化到 DB，支持周期性快照对比
//   - 输出可解释（evidence_tags / evidence_data）
// ============================================================================

// FeedbackLearningService 反馈学习闭环服务
type FeedbackLearningService struct {
	feedbackRepo *repository.FeedbackLearningRepository
	sopAgentRepo *repository.SopAgentRepository
	sopExecRepo  *repository.SopExecutionRepository
}

// NewFeedbackLearningService 创建反馈学习服务
//
// db 参数保留以维持调用方签名兼容；当 db 非空时构造对应 repository，
// 为空时 repos 保持 nil（service 各方法会返回 "db not configured" 错误）。
func NewFeedbackLearningService(db *gorm.DB) *FeedbackLearningService {
	s := &FeedbackLearningService{}
	if db != nil {
		s.feedbackRepo = repository.NewFeedbackLearningRepository(db)
		s.sopAgentRepo = repository.NewSopAgentRepository(db)
		s.sopExecRepo = repository.NewSopExecutionRepository(db)
	}
	return s
}

// ============================================================================
// 能力 1：销冠画像 5 维度提取
// ============================================================================

// ChampionDimensionScore 单维度得分
type ChampionDimensionScore struct {
	Dimension     model.SalesChampionDimension `json:"dimension"`
	Name          string                       `json:"name"`
	Score         float64                      `json:"score"` // 0-100
	SampleCount   int                          `json:"sample_count"`
	PositiveCount int                          `json:"positive_count"`
	NegativeCount int                          `json:"negative_count"`
	EvidenceTags  []string                     `json:"evidence_tags"`
}

// ChampionProfileReport 销冠画像报告（5 维度雷达）
type ChampionProfileReport struct {
	StaffID      uint                     `json:"staff_id"`
	StaffName    string                   `json:"staff_name"`
	Scenario     string                   `json:"scenario"` // ai_champion / staff_xxx
	Dimensions   []ChampionDimensionScore `json:"dimensions"`
	OverallScore float64                  `json:"overall_score"` // 5 维度平均
	PeriodStart  time.Time                `json:"period_start"`
	PeriodEnd    time.Time                `json:"period_end"`
	GeneratedAt  time.Time                `json:"generated_at"`
}

// DimensionDimensionName 维度中文名映射
var dimensionNameMap = map[model.SalesChampionDimension]string{
	model.DimensionObjectionHandling:   "异议处理能力",
	model.DimensionClosingInvitation:   "逼单邀约能力",
	model.DimensionFollowupActivation:  "跟进激活能力",
	model.DimensionNurturingConversion: "培育转化能力",
	model.DimensionRepurchaseOperation: "复购运营能力",
}

// DimensionName 维度中文名
func DimensionName(d model.SalesChampionDimension) string {
	if name, ok := dimensionNameMap[d]; ok {
		return name
	}
	return string(d)
}

// ExtractProfile 提取销冠画像（从对话记录）
// staffID=0 表示系统级 智能体
// scenario 标识场景，如 "ai_champion" 或 "staff_123"
func (s *FeedbackLearningService) ExtractProfile(ctx context.Context, staffID uint, staffName, scenario string, periodStart, periodEnd time.Time) (*ChampionProfileReport, error) {
	if s.feedbackRepo == nil {
		return nil, fmt.Errorf("db not configured")
	}
	if periodEnd.IsZero() {
		periodEnd = time.Now()
	}
	if periodStart.IsZero() {
		periodStart = periodEnd.AddDate(0, 0, -30)
	}

	// 拉取周期内的 AI 回复消息（SenderType=ai）
	messages, err := s.feedbackRepo.ListAIMessagesByPeriod(ctx, periodStart, periodEnd, 5000)
	if err != nil {
		return nil, fmt.Errorf("query ai messages: %w", err)
	}

	// 拉取对应的客户消息（用于上下文判断）
	sessionIDs := extractSessionIDs(messages)
	customerMsgMap, _ := s.queryCustomerMessages(ctx, sessionIDs, periodStart, periodEnd)

	report := &ChampionProfileReport{
		StaffID:     staffID,
		StaffName:   staffName,
		Scenario:    scenario,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		GeneratedAt: time.Now(),
	}

	// 逐维度提取
	for _, dim := range model.AllSalesChampionDimensions {
		score := s.extractDimension(ctx, dim, messages, customerMsgMap)
		score.Name = DimensionName(dim)
		report.Dimensions = append(report.Dimensions, score)
	}

	// 计算综合分
	total := 0.0
	for _, d := range report.Dimensions {
		total += d.Score
	}
	if len(report.Dimensions) > 0 {
		report.OverallScore = total / float64(len(report.Dimensions))
	}

	// 持久化快照
	if err := s.persistProfileSnapshot(ctx, report); err != nil {
		// 持久化失败不阻断主流程，但必须记录错误（避免 _ = err 静默吞噬）
		logger.Errorf("feedback_learning: persistProfileSnapshot failed: %v", err)
	}

	return report, nil
}

// extractDimension 提取单维度得分
func (s *FeedbackLearningService) extractDimension(ctx context.Context, dim model.SalesChampionDimension, messages []model.SessionMessage, customerMsgs map[string][]model.SessionMessage) ChampionDimensionScore {
	score := ChampionDimensionScore{
		Dimension:    dim,
		EvidenceTags: []string{},
	}

	var positive, negative int
	var evidences []string

	for _, msg := range messages {
		ctx := ""
		if msgs, ok := customerMsgs[msg.SessionID]; ok {
			ctx = latestCustomerMessage(msgs, msg.CreatedAt)
		}

		hit, evidence := classifyDimensionHit(dim, msg.Content, ctx)
		if !hit {
			continue
		}
		score.SampleCount++
		if evidence.positive {
			positive++
			evidences = append(evidences, evidence.tag+"✓")
		} else {
			negative++
			evidences = append(evidences, evidence.tag+"✗")
		}
	}

	score.PositiveCount = positive
	score.NegativeCount = negative

	// 证据标签去重（最多 5 个）
	score.EvidenceTags = dedupeTags(evidences, 5)

	// 得分计算：正向 / (正向 + 负向) * 100，无样本给 50（中性）
	if score.SampleCount == 0 {
		score.Score = 50.0
	} else {
		score.Score = float64(positive) / float64(score.SampleCount) * 100
	}
	// 四舍五入到 2 位
	score.Score = roundTo2(score.Score)

	return score
}

// dimensionEvidence 维度证据
type dimensionEvidence struct {
	positive bool
	tag      string
}

// classifyDimensionHit 判断该消息是否命中指定维度
// 返回 (是否命中, 证据)
func classifyDimensionHit(dim model.SalesChampionDimension, aiReply, customerMsg string) (bool, dimensionEvidence) {
	switch dim {
	case model.DimensionObjectionHandling:
		return classifyObjectionHandling(aiReply, customerMsg)
	case model.DimensionClosingInvitation:
		return classifyClosingInvitation(aiReply, customerMsg)
	case model.DimensionFollowupActivation:
		return classifyFollowupActivation(aiReply, customerMsg)
	case model.DimensionNurturingConversion:
		return classifyNurturingConversion(aiReply, customerMsg)
	case model.DimensionRepurchaseOperation:
		return classifyRepurchaseOperation(aiReply, customerMsg)
	}
	return false, dimensionEvidence{}
}

// classifyObjectionHandling 异议处理能力
// 触发：客户表达异议（太贵/不需要/考虑一下/别人家更便宜）
// 正向：AI 回复包含解释/对比/价值塑造/让步
func classifyObjectionHandling(aiReply, customerMsg string) (bool, dimensionEvidence) {
	// 客户异议关键词
	objectionWords := []string{"太贵", "贵了", "不需要", "不用了", "考虑一下", "考虑下",
		"再说吧", "算了", "别人家", "其他家", "竞品", "对比", "不值", "不划算"}
	hasObjection := false
	for _, w := range objectionWords {
		if strings.Contains(customerMsg, w) {
			hasObjection = true
			break
		}
	}
	if !hasObjection {
		return false, dimensionEvidence{}
	}

	// AI 正向处理：解释/对比/价值/让步/限时
	positiveWords := []string{"理解", "明白", "确实", "其实", "性价比", "价值",
		"活动", "优惠", "划算", "对比", "适合", "帮您", "值得", "保障", "服务"}
	negativeWords := []string{"好的，再见", "打扰了", "抱歉打扰", "那算了"}
	for _, w := range positiveWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: true, tag: "异议已回应"}
		}
	}
	for _, w := range negativeWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: false, tag: "异议放弃"}
		}
	}
	// 命中异议但 AI 回复过短或无实质内容
	if len([]rune(aiReply)) < 10 {
		return true, dimensionEvidence{positive: false, tag: "回应过短"}
	}
	return true, dimensionEvidence{positive: true, tag: "异议已回应"}
}

// classifyClosingInvitation 逼单邀约能力
// 触发：AI 主动推进下单/到店/体验/预约
// 正向：客户后续回复包含正向信号（好的/可以/行/试试/预约）
func classifyClosingInvitation(aiReply, customerMsg string) (bool, dimensionEvidence) {
	// AI 逼单/邀约关键词
	closingWords := []string{"下单", "预约", "到店", "体验", "试试", "下单", "购买",
		"付款", "订金", "锁定", "名额", "限时", "今天", "马上", "安排", "帮您下单"}
	hasClosing := false
	for _, w := range closingWords {
		if strings.Contains(aiReply, w) {
			hasClosing = true
			break
		}
	}
	if !hasClosing {
		return false, dimensionEvidence{}
	}

	// 客户正向信号（同一条 AI 回复后的客户反馈，由调用方传入）
	positiveSignals := []string{"好的", "可以", "行", "试试", "预约", "下单", "安排",
		"怎么付", "多少钱", "可以试试", "好"}
	negativeSignals := []string{"不用", "算了", "再看看", "考虑", "不了", "下次"}
	for _, w := range positiveSignals {
		if strings.Contains(customerMsg, w) {
			return true, dimensionEvidence{positive: true, tag: "逼单成功"}
		}
	}
	for _, w := range negativeSignals {
		if strings.Contains(customerMsg, w) {
			return true, dimensionEvidence{positive: false, tag: "逼单被拒"}
		}
	}
	// 无客户反馈，AI 发起逼单算中性正向
	return true, dimensionEvidence{positive: true, tag: "逼单发起"}
}

// classifyFollowupActivation 跟进激活能力
// 触发：AI 对沉默客户的主动跟进
// 正向：跟进消息包含关怀/价值/新信息，避免纯催促
func classifyFollowupActivation(aiReply, customerMsg string) (bool, dimensionEvidence) {
	// 跟进关键词（AI 主动发起）
	followupWords := []string{"好久没", "最近", "在吗", "打扰了", "想您", "想到您",
		"更新", "新品", "到货", "上架", "活动", "优惠", "福利",
		"怎么不回", "怎么不", "赶紧", "还不", "为什么不回"}
	hasFollowup := false
	for _, w := range followupWords {
		if strings.Contains(aiReply, w) {
			hasFollowup = true
			break
		}
	}
	if !hasFollowup {
		return false, dimensionEvidence{}
	}

	// 正向：带价值/关怀，非纯催促
	positiveWords := []string{"活动", "优惠", "新品", "到货", "福利", "适合", "建议", "帮您"}
	negativeWords := []string{"催", "怎么不回", "为什么不", "还不", "赶紧"}
	for _, w := range positiveWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: true, tag: "价值跟进"}
		}
	}
	for _, w := range negativeWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: false, tag: "催促式跟进"}
		}
	}
	return true, dimensionEvidence{positive: true, tag: "常规跟进"}
}

// classifyNurturingConversion 培育转化能力
// 触发：长期培育场景（多次互动后的转化推进）
// 正向：AI 回复体现对客户需求的持续理解 + 推进
func classifyNurturingConversion(aiReply, customerMsg string) (bool, dimensionEvidence) {
	// 培育关键词
	nurturingWords := []string{"之前", "上次", "记得", "了解您", "根据您", "为您",
		"推荐", "适合", "专属", "定制", "方案"}
	hasNurturing := false
	for _, w := range nurturingWords {
		if strings.Contains(aiReply, w) {
			hasNurturing = true
			break
		}
	}
	if !hasNurturing {
		return false, dimensionEvidence{}
	}

	// 正向：推进转化 + 个性化
	positiveWords := []string{"推荐", "适合", "专属", "方案", "建议", "帮您", "安排"}
	for _, w := range positiveWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: true, tag: "个性化培育"}
		}
	}
	return true, dimensionEvidence{positive: false, tag: "培育无推进"}
}

// classifyRepurchaseOperation 复购运营能力
// 触发：老客复购/增购/推荐场景
// 正向：AI 主动提及复购/老客权益/推荐奖励
func classifyRepurchaseOperation(aiReply, customerMsg string) (bool, dimensionEvidence) {
	repurchaseWords := []string{"老客", "老朋友", "回购", "复购", "再次", "回头",
		"推荐", "分享", "老用户", "会员", "积分", "专属"}
	hasRepurchase := false
	for _, w := range repurchaseWords {
		if strings.Contains(aiReply, w) {
			hasRepurchase = true
			break
		}
	}
	if !hasRepurchase {
		return false, dimensionEvidence{}
	}

	positiveWords := []string{"会员", "积分", "专属", "优惠", "福利", "老朋友", "感谢"}
	for _, w := range positiveWords {
		if strings.Contains(aiReply, w) {
			return true, dimensionEvidence{positive: true, tag: "老客权益"}
		}
	}
	return true, dimensionEvidence{positive: true, tag: "复购触达"}
}

// ============================================================================
// 能力 2：SOP 节点转化率分析
// ============================================================================

// SOPNodeConversionStats SOP 节点转化率统计
type SOPNodeConversionStats struct {
	SOPID             uint                   `json:"sop_id"`
	SOPName           string                 `json:"sop_name"`
	Variant           string                 `json:"variant"` // A/B 测试 variant
	Nodes             []NodeConversionDetail `json:"nodes"`
	TotalExecutions   int64                  `json:"total_executions"`
	OverallConversion float64                `json:"overall_conversion"` // 端到端转化率
	GeneratedAt       time.Time              `json:"generated_at"`
}

// NodeConversionDetail 节点转化详情
type NodeConversionDetail struct {
	NodeID         string  `json:"node_id"`
	NodeName       string  `json:"node_name"`
	NodeType       string  `json:"node_type"`
	EnteredCount   int     `json:"entered_count"`   // 进入该节点的执行数
	SuccessCount   int     `json:"success_count"`   // 成功推进数
	AbandonedCount int     `json:"abandoned_count"` // 流失数
	FailedCount    int     `json:"failed_count"`    // 失败数
	ConversionRate float64 `json:"conversion_rate"` // 转化率 = success / entered * 100
	DropRate       float64 `json:"drop_rate"`       // 流失率 = abandoned / entered * 100
	AvgDurationMs  int     `json:"avg_duration_ms"` // 平均停留时长
	IsBottleneck   bool    `json:"is_bottleneck"`   // 是否瓶颈节点
}

// AnalyzeNodeConversion 分析 SOP 节点转化率
func (s *FeedbackLearningService) AnalyzeNodeConversion(ctx context.Context, sopID uint, variant string) (*SOPNodeConversionStats, error) {
	if s.feedbackRepo == nil {
		return nil, fmt.Errorf("db not configured")
	}

	// 查询 SOP 信息
	agent, err := s.sopAgentRepo.GetByID(ctx, sopID)
	if err != nil {
		return nil, fmt.Errorf("query sop: %w", err)
	}

	// 查询节点流转记录
	transitions, err := s.feedbackRepo.ListNodeTransitionsBySOPAndVariant(ctx, sopID, variant)
	if err != nil {
		return nil, fmt.Errorf("query transitions: %w", err)
	}

	stats := &SOPNodeConversionStats{
		SOPID:       sopID,
		SOPName:     agent.Name,
		Variant:     variant,
		GeneratedAt: time.Now(),
	}

	// 按节点聚合
	nodeMap := make(map[string]*NodeConversionDetail)
	for _, t := range transitions {
		node := nodeMap[t.ToNode]
		if node == nil {
			node = &NodeConversionDetail{
				NodeID:   t.ToNode,
				NodeType: t.NodeType,
			}
			nodeMap[t.ToNode] = node
		}
		node.EnteredCount++
		switch t.Outcome {
		case model.NodeOutcomeSuccess:
			node.SuccessCount++
		case model.NodeOutcomeAbandoned:
			node.AbandonedCount++
		case model.NodeOutcomeFailed:
			node.FailedCount++
		}
		node.AvgDurationMs = (node.AvgDurationMs*(node.EnteredCount-1) + t.DurationMs) / node.EnteredCount
	}

	// 计算转化率 + 识别瓶颈
	for _, node := range nodeMap {
		if node.EnteredCount > 0 {
			node.ConversionRate = float64(node.SuccessCount) / float64(node.EnteredCount) * 100
			node.DropRate = float64(node.AbandonedCount) / float64(node.EnteredCount) * 100
		}
		// 瓶颈节点：转化率 < 50% 且样本数 >= 5
		if node.ConversionRate < 50 && node.EnteredCount >= 5 {
			node.IsBottleneck = true
		}
		node.ConversionRate = roundTo2(node.ConversionRate)
		node.DropRate = roundTo2(node.DropRate)
		stats.Nodes = append(stats.Nodes, *node)
	}

	// 总执行数（忽略 error，保持等价）
	stats.TotalExecutions, _ = s.sopExecRepo.CountBySOPID(ctx, sopID)

	// 端到端转化率
	if stats.TotalExecutions > 0 {
		successCount, _ := s.sopExecRepo.CountBySOPIDAndStatus(ctx, sopID, SOPStatusSuccess)
		stats.OverallConversion = float64(successCount) / float64(stats.TotalExecutions) * 100
		stats.OverallConversion = roundTo2(stats.OverallConversion)
	}

	return stats, nil
}

// ============================================================================
// 能力 3：低转化节点优化建议生成
// ============================================================================

// OptimizationSuggestionInput 优化建议生成输入
type OptimizationSuggestionInput struct {
	SOPID             uint
	SOPName           string
	NodeConversion    *SOPNodeConversionStats
	LowScoreThreshold float64 // 转化率低于此值生成建议（默认 50）
	MinSampleCount    int     // 最小样本数（默认 5）
}

// GenerateOptimizationSuggestions 为低转化节点生成优化建议
func (s *FeedbackLearningService) GenerateOptimizationSuggestions(ctx context.Context, input OptimizationSuggestionInput) ([]model.OptimizationSuggestion, error) {
	if s.feedbackRepo == nil {
		return nil, fmt.Errorf("db not configured")
	}
	if input.NodeConversion == nil {
		return nil, fmt.Errorf("node conversion stats required")
	}
	threshold := input.LowScoreThreshold
	if threshold <= 0 {
		threshold = 50
	}
	minSample := input.MinSampleCount
	if minSample <= 0 {
		minSample = 5
	}

	var suggestions []model.OptimizationSuggestion
	for _, node := range input.NodeConversion.Nodes {
		// 只对低转化 + 足够样本的节点生成建议
		if node.ConversionRate >= threshold || node.EnteredCount < minSample {
			continue
		}
		sug := buildOptimizationSuggestion(input.SOPID, input.SOPName, node, threshold)
		suggestions = append(suggestions, sug)
	}

	// 持久化：P1-17 修复，原循环单条 Create 改为 CreateInBatches 批写
	if len(suggestions) > 0 {
		if err := s.feedbackRepo.CreateSuggestionsInBatches(ctx, suggestions, 100); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Int("count", len(suggestions)).
				Msg("[feedback] batch insert suggestions failed")
		}
	}

	return suggestions, nil
}

// buildOptimizationSuggestion 根据节点类型和问题生成建议
func buildOptimizationSuggestion(sopID uint, sopName string, node NodeConversionDetail, threshold float64) model.OptimizationSuggestion {
	sug := model.OptimizationSuggestion{
		SOPID:        sopID,
		SOPName:      sopName,
		NodeID:       node.NodeID,
		NodeName:     node.NodeName,
		NodeType:     node.NodeType,
		CurrentScore: node.ConversionRate,
		Threshold:    threshold,
		SampleCount:  node.EnteredCount,
		Status:       model.SuggestionStatusPending,
	}

	// 根据节点类型和流失情况生成建议
	switch node.NodeType {
	case "llm":
		sug.SuggestionType = model.SuggestionTypePromptRewrite
		sug.Priority = 2
		sug.SuggestionText = fmt.Sprintf(
			"LLM 节点 [%s] 转化率 %.1f%%（低于阈值 %.0f%%），流失 %d 人。"+
				"建议优化 prompt：1) 增加共情话术；2) 明确下一步行动号召；3) 精简回复长度。",
			node.NodeName, node.ConversionRate, threshold, node.AbandonedCount)
		sug.ExpectedImpact = "预计提升转化率 15-25%"
	case "condition":
		sug.SuggestionType = model.SuggestionTypeBranchPrune
		sug.Priority = 1
		sug.SuggestionText = fmt.Sprintf(
			"条件节点 [%s] 转化率 %.1f%%（低于阈值 %.0f%%），流失 %d 人。"+
				"建议审查分支条件：1) 简化条件判断；2) 增加兜底分支；3) 调整分支优先级。",
			node.NodeName, node.ConversionRate, threshold, node.AbandonedCount)
		sug.ExpectedImpact = "预计减少流失 20-30%"
	case "action":
		if node.AbandonedCount > node.FailedCount {
			sug.SuggestionType = model.SuggestionTypeTimingAdjust
			sug.Priority = 1
			sug.SuggestionText = fmt.Sprintf(
				"动作节点 [%s] 流失率 %.1f%%。建议调整触达时机或频率，避免过度打扰。",
				node.NodeName, node.DropRate)
		} else {
			sug.SuggestionType = model.SuggestionTypeNodeMerge
			sug.Priority = 0
			sug.SuggestionText = fmt.Sprintf(
				"动作节点 [%s] 失败 %d 次。建议检查动作配置或合并相邻动作节点。",
				node.NodeName, node.FailedCount)
		}
		sug.ExpectedImpact = "预计提升稳定性 10-20%"
	default:
		// 通用建议
		sug.SuggestionType = model.SuggestionTypeAddObjection
		sug.Priority = 0
		sug.SuggestionText = fmt.Sprintf(
			"节点 [%s] 转化率 %.1f%%。建议补充异议处理分支和共情话术。",
			node.NodeName, node.ConversionRate)
		sug.ExpectedImpact = "预计提升转化率 5-15%"
	}

	// 证据数据
	sug.EvidenceData = map[string]any{
		"entered_count":   node.EnteredCount,
		"success_count":   node.SuccessCount,
		"abandoned_count": node.AbandonedCount,
		"failed_count":    node.FailedCount,
		"conversion_rate": node.ConversionRate,
		"drop_rate":       node.DropRate,
		"is_bottleneck":   node.IsBottleneck,
	}

	return sug
}

// ============================================================================
// 辅助方法
// ============================================================================

// RecordNodeTransition 记录节点流转
// 供 SalesEngine / SOPService 在执行 SOP 时调用
func (s *FeedbackLearningService) RecordNodeTransition(ctx context.Context, t *model.SOPNodeTransition) error {
	if s.feedbackRepo == nil {
		return nil
	}
	return s.feedbackRepo.CreateNodeTransition(ctx, t)
}

// ListPendingSuggestions 列出待审核建议
func (s *FeedbackLearningService) ListPendingSuggestions(ctx context.Context, sopID uint, limit int) ([]model.OptimizationSuggestion, error) {
	if s.feedbackRepo == nil {
		return nil, fmt.Errorf("db not configured")
	}
	list, err := s.feedbackRepo.ListPendingSuggestions(ctx, sopID, limit)
	if err != nil {
		return nil, fmt.Errorf("list suggestions: %w", err)
	}
	return list, nil
}

// ReviewSuggestion 审核建议
func (s *FeedbackLearningService) ReviewSuggestion(ctx context.Context, suggestionID uint, reviewerID uint, action string) error {
	if s.feedbackRepo == nil {
		return fmt.Errorf("db not configured")
	}
	now := time.Now()
	updates := map[string]any{
		"reviewed_by": reviewerID,
		"reviewed_at": &now,
	}
	switch action {
	case "approve":
		updates["status"] = model.SuggestionStatusApproved
	case "reject":
		updates["status"] = model.SuggestionStatusRejected
	case "apply":
		updates["status"] = model.SuggestionStatusApplied
		updates["applied_at"] = &now
	default:
		return fmt.Errorf("invalid action: %s (expected approve/reject/apply)", action)
	}
	return s.feedbackRepo.UpdateSuggestionFields(ctx, suggestionID, updates)
}

// GetLatestProfile 获取最新画像快照
func (s *FeedbackLearningService) GetLatestProfile(ctx context.Context, staffID uint, scenario string) ([]model.SalesChampionProfileSnapshot, error) {
	if s.feedbackRepo == nil {
		return nil, fmt.Errorf("db not configured")
	}
	list, err := s.feedbackRepo.ListLatestProfileSnapshots(ctx, staffID, scenario, 5) // 每维度最新 1 条，共 5 条
	if err != nil {
		return nil, fmt.Errorf("query profile: %w", err)
	}
	return list, nil
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// persistProfileSnapshot 持久化画像快照
func (s *FeedbackLearningService) persistProfileSnapshot(ctx context.Context, report *ChampionProfileReport) error {
	for _, dim := range report.Dimensions {
		snapshot := model.SalesChampionProfileSnapshot{
			StaffID:       report.StaffID,
			StaffName:     report.StaffName,
			Scenario:      report.Scenario,
			Dimension:     string(dim.Dimension),
			Score:         dim.Score,
			SampleCount:   dim.SampleCount,
			PositiveCount: dim.PositiveCount,
			NegativeCount: dim.NegativeCount,
			EvidenceTags:  toStringArray(dim.EvidenceTags),
			PeriodStart:   report.PeriodStart,
			PeriodEnd:     report.PeriodEnd,
		}
		if err := s.feedbackRepo.CreateProfileSnapshot(ctx, &snapshot); err != nil {
			return fmt.Errorf("persist snapshot: %w", err)
		}
	}
	return nil
}

// queryCustomerMessages 查询客户消息（按 session 分组）
func (s *FeedbackLearningService) queryCustomerMessages(ctx context.Context, sessionIDs []string, start, end time.Time) (map[string][]model.SessionMessage, error) {
	if len(sessionIDs) == 0 {
		return map[string][]model.SessionMessage{}, nil
	}
	msgs, err := s.feedbackRepo.ListCustomerMessagesBySessions(ctx, sessionIDs, start, end)
	if err != nil {
		return nil, err
	}
	m := make(map[string][]model.SessionMessage)
	for _, msg := range msgs {
		m[msg.SessionID] = append(m[msg.SessionID], msg)
	}
	return m, nil
}

// extractSessionIDs 提取 session ID 列表（去重）
func extractSessionIDs(messages []model.SessionMessage) []string {
	seen := map[string]bool{}
	var ids []string
	for _, m := range messages {
		if !seen[m.SessionID] {
			seen[m.SessionID] = true
			ids = append(ids, m.SessionID)
		}
	}
	return ids
}

// latestCustomerMessage 获取指定时间点前最近的一条客户消息
func latestCustomerMessage(msgs []model.SessionMessage, before time.Time) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].CreatedAt.Before(before) || msgs[i].CreatedAt.Equal(before) {
			return msgs[i].Content
		}
	}
	return ""
}

// dedupeTags 标签去重（限制数量）
func dedupeTags(tags []string, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
			if len(out) >= max {
				break
			}
		}
	}
	return out
}

// toStringArray []string → JSONArray
func toStringArray(items []string) model.JSONArray {
	if len(items) == 0 {
		return model.JSONArray{}
	}
	out := make(model.JSONArray, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
}

// roundTo2 四舍五入到 2 位小数
func roundTo2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
