package service

// intent_recognition_fine.go 精细意图识别（8 大类 + 7 子类）
//
// 五层架构归属: L2 服务层 / L3 编排层
// 设计依据: PRD §M-2 P1 缺口修复
// 私域独立部署: 无 merchant_id 字段
//
// 功能：
//   - 8 大意图类：consult / price_inquiry / objection / after_sale / complaint / churn / intent_buy / ask_product
//   - 7 子类细分（每个大类下的子类，共 26 个子类）
//   - 规则匹配（快速，毫秒级）
//   - LLM 识别（慢速，confidence < 0.6 触发二次识别）
//   - IntentLog 持久化（intent_logs 表）
//   - API：POST /api/intent/recognize、GET /api/intent/logs、GET /api/intent/stats
//
// 与旧版 IntentRecognizer 的关系：
//   - 旧版 Recognize 方法保留（兼容现有调用方）
//   - 新版 RecognizeIntent 方法返回 IntentResult（含 major/minor）
//   - 新旧方法共享 dispatcher/db/cache 依赖

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ===== 8 大意图类常量 =====

const (
	IntentMajorConsult      = "consult"       // 咨询
	IntentMajorPriceInquiry = "price_inquiry" // 询价
	IntentMajorObjection    = "objection"     // 异议
	IntentMajorAfterSale    = "after_sale"    // 售后
	IntentMajorComplaint    = "complaint"     // 投诉
	IntentMajorChurn        = "churn"         // 流失
	IntentMajorIntentBuy    = "intent_buy"    // 购买意向
	IntentMajorAskProduct   = "ask_product"   // 产品咨询
)

// ===== 7 子类常量（每个大类下的细分） =====

// consult 子类
const (
	IntentMinorConsultGeneral         = "general"          // 一般咨询
	IntentMinorConsultProductSpecific = "product_specific" // 产品特定咨询
	IntentMinorConsultComparison      = "comparison"       // 对比咨询
)

// price_inquiry 子类
const (
	IntentMinorPriceBudgetCheck  = "budget_check"     // 预算确认
	IntentMinorPriceDiscountReq  = "discount_request" // 折扣请求
	IntentMinorPricePaymentTerms = "payment_terms"    // 付款方式
)

// objection 子类
const (
	IntentMinorObjectionPriceHigh     = "price_too_high"        // 价格太高
	IntentMinorObjectionTrustIssue    = "trust_issue"           // 信任问题
	IntentMinorObjectionTimingBad     = "timing_bad"            // 时机不对
	IntentMinorObjectionCompetitorCmp = "competitor_comparison" // 竞品对比
)

// after_sale 子类
const (
	IntentMinorAfterSaleQuality  = "quality_issue"  // 质量问题
	IntentMinorAfterSaleDelivery = "delivery_issue" // 配送问题
	IntentMinorAfterSaleRefund   = "refund_request" // 退款请求
	IntentMinorAfterSaleWarranty = "warranty"       // 保修
)

// complaint 子类
const (
	IntentMinorComplaintService = "service_complaint" // 服务投诉
	IntentMinorComplaintProduct = "product_complaint" // 产品投诉
	IntentMinorComplaintBilling = "billing_complaint" // 账单投诉
)

// churn 子类
const (
	IntentMinorChurnCancelSub  = "cancel_subscription" // 取消订阅
	IntentMinorChurnSwitchComp = "switch_competitor"   // 转投竞品
	IntentMinorChurnStopUsing  = "stop_using"          // 停止使用
)

// intent_buy 子类
const (
	IntentMinorIntentBuyReady    = "ready_to_buy"   // 准备购买
	IntentMinorIntentBuyNeedInfo = "need_more_info" // 需要更多信息
	IntentMinorIntentBuyApproval = "need_approval"  // 需要审批
)

// ask_product 子类
const (
	IntentMinorAskProductFeature = "feature_query" // 功能查询
	IntentMinorAskProductAvail   = "availability"  // 可用性
	IntentMinorAskProductSpec    = "spec_query"    // 规格查询
)

// IntentResult 精细意图识别结果
type IntentResult struct {
	Major      string  `json:"major"`                // 8 大意图类之一
	Minor      string  `json:"minor"`                // 子类细分
	Confidence float64 `json:"confidence"`           // 0.0-1.0
	Method     string  `json:"method"`               // rule / llm / hybrid
	Reasoning  string  `json:"reasoning,omitempty"`  // LLM 推理过程（method=llm 时填充）
	LatencyMs  int     `json:"latency_ms,omitempty"` // 识别耗时
	LLMModel   string  `json:"llm_model,omitempty"`  // LLM 模型（method=llm 时填充）
}

// minorRule 子类规则定义
type minorRule struct {
	minor    string
	keywords []string
	weight   int // 关键词权重（用于排序）
}

// majorRule 大类规则定义
type majorRule struct {
	major    string
	minors   []minorRule
	keywords []string // 大类通用关键词（无子类匹配时使用）
}

// fineIntentRules 精细意图规则表
// 按 major 分组，每个 major 下含若干 minor 子类规则
var fineIntentRules = []majorRule{
	{
		major:    IntentMajorConsult,
		keywords: []string{"咨询", "了解一下", "请问", "想问", "请问一下", "想了解", "怎么处理", "怎么回事"},
		minors: []minorRule{
			{minor: IntentMinorConsultGeneral, keywords: []string{"咨询", "了解一下", "想了解", "请问"}, weight: 1},
			{minor: IntentMinorConsultProductSpecific, keywords: []string{"这个产品", "这个功能", "这个服务", "这个方案", "你们的产品"}, weight: 2},
			{minor: IntentMinorConsultComparison, keywords: []string{"对比", "比较", "区别", "哪个好", "差异", "优缺点", "对比一下", "两款"}, weight: 4},
		},
	},
	{
		major:    IntentMajorPriceInquiry,
		keywords: []string{"多少钱", "价格", "报价", "价位", "怎么卖", "怎么收费", "费用"},
		minors: []minorRule{
			{minor: IntentMinorPriceBudgetCheck, keywords: []string{"预算", "多少钱", "价位", "什么价位"}, weight: 2},
			{minor: IntentMinorPriceDiscountReq, keywords: []string{"优惠", "折扣", "便宜", "便宜点", "减免", "满减", "活动"}, weight: 3},
			{minor: IntentMinorPricePaymentTerms, keywords: []string{"付款", "分期", "月付", "年付", "支付方式", "结算", "结账"}, weight: 3},
		},
	},
	{
		major:    IntentMajorObjection,
		keywords: []string{"不需要", "太贵", "再考虑", "再说", "不放心", "别家"},
		minors: []minorRule{
			{minor: IntentMinorObjectionPriceHigh, keywords: []string{"太贵", "价格高", "有点高", "不值", "不划算", "贵了"}, weight: 3},
			{minor: IntentMinorObjectionTrustIssue, keywords: []string{"不放心", "骗人", "假的", "靠谱吗", "真的吗", "信不过", "能信吗"}, weight: 3},
			{minor: IntentMinorObjectionTimingBad, keywords: []string{"再说", "过段时间", "等以后", "不急", "等几天", "最近忙", "没时间"}, weight: 3},
			{minor: IntentMinorObjectionCompetitorCmp, keywords: []string{"别的家", "别家", "竞品", "其他品牌", "友商", "对比一下", "比别家", "他家"}, weight: 4},
		},
	},
	{
		major:    IntentMajorAfterSale,
		keywords: []string{"售后", "坏了", "有问题", "故障", "退货", "退款"},
		minors: []minorRule{
			{minor: IntentMinorAfterSaleQuality, keywords: []string{"坏了", "故障", "用不了", "不能用了", "质量问题", "有问题", "出问题"}, weight: 3},
			{minor: IntentMinorAfterSaleDelivery, keywords: []string{"发货", "物流", "快递", "多久到", "没收到", "配送", "运送"}, weight: 3},
			{minor: IntentMinorAfterSaleRefund, keywords: []string{"退货", "退款", "退钱", "退换", "换货", "要退货"}, weight: 3},
			{minor: IntentMinorAfterSaleWarranty, keywords: []string{"保修", "维修", "保养", "质保", "三包"}, weight: 3},
		},
	},
	{
		major:    IntentMajorComplaint,
		keywords: []string{"投诉", "差评", "举报", "气愤", "愤怒"},
		minors: []minorRule{
			{minor: IntentMinorComplaintService, keywords: []string{"服务态度", "客服态度", "处理慢", "没人理", "不负责", "服务差"}, weight: 3},
			{minor: IntentMinorComplaintProduct, keywords: []string{"产品差", "垃圾产品", "质量差", "不能用", "假货", "次品"}, weight: 3},
			{minor: IntentMinorComplaintBilling, keywords: []string{"多扣钱", "乱扣费", "账单错", "费用不对", "计费错", "重复扣款"}, weight: 3},
		},
	},
	{
		major:    IntentMajorChurn,
		keywords: []string{"拉黑", "删除", "退订", "别再发", "取消"},
		minors: []minorRule{
			{minor: IntentMinorChurnCancelSub, keywords: []string{"取消订阅", "取消会员", "退订", "取消续费", "解约"}, weight: 3},
			{minor: IntentMinorChurnSwitchComp, keywords: []string{"换别家", "用竞品", "转其他", "投奔", "换品牌", "别家更好"}, weight: 3},
			{minor: IntentMinorChurnStopUsing, keywords: []string{"不用了", "停用", "拉黑", "删除", "别再发", "再发举报", "屏蔽"}, weight: 3},
		},
	},
	{
		major:    IntentMajorIntentBuy,
		keywords: []string{"怎么买", "下单", "购买", "付款", "要了"},
		minors: []minorRule{
			{minor: IntentMinorIntentBuyReady, keywords: []string{"下单", "购买", "怎么买", "怎么付款", "要了", "来一个", "买了"}, weight: 3},
			{minor: IntentMinorIntentBuyNeedInfo, keywords: []string{"还需要", "想确认", "再了解一下", "更多信息", "具体情况"}, weight: 2},
			{minor: IntentMinorIntentBuyApproval, keywords: []string{"审批", "请示", "汇报", "老板同意", "领导同意", "走流程"}, weight: 3},
		},
	},
	{
		major:    IntentMajorAskProduct,
		keywords: []string{"功能", "效果", "怎么用", "规格", "参数"},
		minors: []minorRule{
			{minor: IntentMinorAskProductFeature, keywords: []string{"功能", "特点", "能做什么", "支持什么", "效果"}, weight: 3},
			{minor: IntentMinorAskProductAvail, keywords: []string{"有货", "现货", "什么时候有", "能买吗", "可售", "库存"}, weight: 3},
			{minor: IntentMinorAskProductSpec, keywords: []string{"规格", "参数", "型号", "尺寸", "容量", "配置"}, weight: 3},
		},
	},
}

// minorLLMThreshold 触发 LLM 二次识别的置信度阈值
const minorLLMThreshold = 0.6

// RecognizeIntent 精细意图识别入口
//
// 流程：
//  1. 规则匹配（快速）：返回 IntentResult（method=rule）
//  2. 若 confidence < 0.6 且 dispatcher 可用：触发 LLM 二次识别（method=hybrid）
//  3. 若规则匹配为空：直接走 LLM 识别（method=llm）
//  4. 持久化 IntentLog
//  5. 返回最终结果
func (s *IntentRecognizer) RecognizeIntent(ctx context.Context, message, customerID, sessionID string) (*IntentResult, error) {
	start := time.Now()
	if message == "" {
		return &IntentResult{
			Major:      IntentMajorConsult,
			Minor:      IntentMinorConsultGeneral,
			Confidence: 0.3,
			Method:     "rule",
		}, nil
	}

	// 1. 规则匹配
	ruleResult := s.recognizeFineByRule(ctx, message)

	var final *IntentResult
	if ruleResult != nil {
		final = ruleResult
		// 2. confidence < 0.6 触发 LLM 二次识别
		if ruleResult.Confidence < minorLLMThreshold && s.dispatcher != nil {
			if llmResult, err := s.recognizeFineByLLM(ctx, message); err == nil && llmResult != nil {
				llmResult.Method = "hybrid"
				final = llmResult
			}
		}
	} else if s.dispatcher != nil {
		// 3. 规则未命中，直接 LLM 识别
		if llmResult, err := s.recognizeFineByLLM(ctx, message); err == nil && llmResult != nil {
			final = llmResult
		}
	}

	// 兜底
	if final == nil {
		final = &IntentResult{
			Major:      IntentMajorConsult,
			Minor:      IntentMinorConsultGeneral,
			Confidence: 0.3,
			Method:     "rule",
		}
	}

	final.LatencyMs = int(time.Since(start).Milliseconds())

	// 4. 持久化 IntentLog
	s.saveIntentLog(ctx, customerID, sessionID, message, final)

	// 5. 触发 SOP 联动（复用旧版逻辑：用 Major 作为 intent_type）
	if customerID != "" {
		s.triggerSOPByIntent(ctx, customerID, sessionID, final.Major, final.Confidence)
	}

	return final, nil
}

// recognizeFineByRule 精细意图规则匹配
// 优先匹配子类关键词，未命中子类则匹配大类通用关键词
func (s *IntentRecognizer) recognizeFineByRule(ctx context.Context, text string)  *IntentResult {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var (
		bestMajor string
		bestMinor string
		bestScore int
	)

	for _, mr := range fineIntentRules {
		// 先匹配子类
		for _, mi := range mr.minors {
			score := 0
			for _, kw := range mi.keywords {
				if strings.Contains(text, kw) {
					score += mi.weight * len(kw)
				}
			}
			if score > bestScore {
				bestScore = score
				bestMajor = mr.major
				bestMinor = mi.minor
			}
		}
		// 子类未命中但大类通用关键词命中
		if bestMajor == "" {
			majorScore := 0
			for _, kw := range mr.keywords {
				if strings.Contains(text, kw) {
					majorScore += len(kw)
				}
			}
			if majorScore > bestScore {
				bestScore = majorScore
				bestMajor = mr.major
				// 大类命中但无具体子类，使用第一个子类作为默认
				if len(mr.minors) > 0 {
					bestMinor = mr.minors[0].minor
				}
			}
		}
	}

	if bestMajor == "" || bestScore == 0 {
		return nil
	}

	// 置信度计算：score 越高 confidence 越高，上限 0.92
	conf := 0.5 + float64(bestScore)*0.03
	if conf > 0.92 {
		conf = 0.92
	}

	return &IntentResult{
		Major:      bestMajor,
		Minor:      bestMinor,
		Confidence: conf,
		Method:     "rule",
	}
}

// recognizeFineByLLM LLM 精细意图识别
func (s *IntentRecognizer) recognizeFineByLLM(ctx context.Context, text string) (*IntentResult, error) {
	if s.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher is nil")
	}

	// 构建 8 大类 + 7 子类的提示词
	var sb strings.Builder
	sb.WriteString("以下是 8 大意图类及其子类，请从其中选择最匹配的 1 个大类和 1 个子类：\n")
	for _, mr := range fineIntentRules {
		sb.WriteString(fmt.Sprintf("- %s\n", mr.major))
		for _, mi := range mr.minors {
			sb.WriteString(fmt.Sprintf("  - %s\n", mi.minor))
		}
	}

	prompt := fmt.Sprintf(`你是销售对话意图识别专家。分析以下客户消息，从给定意图列表中选择最匹配的 1 个大类和 1 个子类。

【客户消息】: %s

【意图列表】:
%s

【输出要求】(严格按 JSON 格式输出，不要添加任何其他内容):
{
  "major": "8 大类之一",
  "minor": "对应子类之一",
  "confidence": 0.0-1.0 的置信度,
  "reasoning": "简短解释为何选择此意图（不超过 50 字）"
}`, text, sb.String())

	result, err := s.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioIntentRecognize,
		Prompt:      prompt,
		JSONMode:    true,
		MaxTokens:   300,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("llm dispatch: %w", err)
	}

	var parsed struct {
		Major      string  `json:"major"`
		Minor      string  `json:"minor"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromStr(result.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("parse llm intent: %w", err)
	}

	// 校验 major/minor 合法性，非法时回退到 consult/general
	if !isValidMajor(parsed.Major) {
		parsed.Major = IntentMajorConsult
		parsed.Minor = IntentMinorConsultGeneral
	} else if !isValidMinor(parsed.Major, parsed.Minor) {
		// major 合法但 minor 非法：使用该 major 的第一个子类
		parsed.Minor = getDefaultMinor(parsed.Major)
	}

	if parsed.Confidence <= 0 {
		parsed.Confidence = 0.5
	}
	if parsed.Confidence > 1 {
		parsed.Confidence = 1
	}

	return &IntentResult{
		Major:      parsed.Major,
		Minor:      parsed.Minor,
		Confidence: parsed.Confidence,
		Method:     "llm",
		Reasoning:  parsed.Reasoning,
		LLMModel:   result.Model,
	}, nil
}

// isValidMajor 判断是否合法的大类
func isValidMajor(major string) bool {
	for _, mr := range fineIntentRules {
		if mr.major == major {
			return true
		}
	}
	return false
}

// isValidMinor 判断 minor 是否属于指定 major
func isValidMinor(major, minor string) bool {
	for _, mr := range fineIntentRules {
		if mr.major == major {
			for _, mi := range mr.minors {
				if mi.minor == minor {
					return true
				}
			}
		}
	}
	return false
}

// getDefaultMinor 获取大类的默认子类（第一个）
func getDefaultMinor(major string) string {
	for _, mr := range fineIntentRules {
		if mr.major == major && len(mr.minors) > 0 {
			return mr.minors[0].minor
		}
	}
	return IntentMinorConsultGeneral
}

// saveIntentLog 持久化 IntentLog
func (s *IntentRecognizer) saveIntentLog(ctx context.Context, customerID, sessionID, message string, result *IntentResult) {
	if s.db == nil || result == nil {
		return
	}
	log := &model.IntentLog{
		CustomerID:  customerID,
		SessionID:   sessionID,
		Message:     message,
		IntentMajor: result.Major,
		IntentMinor: result.Minor,
		Confidence:  result.Confidence,
		Method:      result.Method,
		LatencyMs:   result.LatencyMs,
		Reasoning:   result.Reasoning,
		Timestamp:   time.Now(),
	}
	// 异步落库 + panic recover（与旧版 saveRecord 一致）
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("intent_recognition_fine: async persist recovered from panic: %v", r)
			}
		}()
		if err := s.db.Create(log).Error; err != nil {
			logger.Errorf("intent_recognition_fine: async persist intent log failed: %v", err)
		}
	}()
}

// GetIntentLogs 查询意图识别日志
//
// 参数：
//   - customerID: 客户 ID（可选，空表示不限）
//   - major: 大类过滤（可选，空表示不限）
//   - limit: 返回条数上限
func (s *IntentRecognizer) GetIntentLogs(ctx context.Context, customerID, major string, limit int)  ([]model.IntentLog, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := s.db.Model(&model.IntentLog{})
	if customerID != "" {
		q = q.Where("customer_id = ?", customerID)
	}
	if major != "" {
		q = q.Where("intent_major = ?", major)
	}
	var logs []model.IntentLog
	err := q.Order("timestamp DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// GetIntentLogStats 意图识别统计（按 major + minor 聚合）
//
// 参数：
//   - days: 统计最近 N 天的数据
func (s *IntentRecognizer) GetIntentLogStats(ctx context.Context, days int)  (map[string]any, error) {
	if s.db == nil {
		return nil, nil
	}
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	// 按 major 聚合
	type majorStat struct {
		IntentMajor string  `json:"intent_major"`
		Count       int64   `json:"count"`
		AvgConf     float64 `json:"avg_confidence"`
	}
	var majorStats []majorStat
	if err := s.db.Model(&model.IntentLog{}).
		Select("intent_major, COUNT(*) as count, AVG(confidence) as avg_conf").
		Where("timestamp > ?", since).
		Group("intent_major").
		Scan(&majorStats).Error; err != nil {
		return nil, err
	}

	// 按 minor 聚合
	type minorStat struct {
		IntentMinor string `json:"intent_minor"`
		IntentMajor string `json:"intent_major"`
		Count       int64  `json:"count"`
	}
	var minorStats []minorStat
	if err := s.db.Model(&model.IntentLog{}).
		Select("intent_major, intent_minor, COUNT(*) as count").
		Where("timestamp > ?", since).
		Group("intent_major, intent_minor").
		Scan(&minorStats).Error; err != nil {
		return nil, err
	}

	// 按 method 聚合
	type methodStat struct {
		Method string `json:"method"`
		Count  int64  `json:"count"`
	}
	var methodStats []methodStat
	if err := s.db.Model(&model.IntentLog{}).
		Select("method, COUNT(*) as count").
		Where("timestamp > ?", since).
		Group("method").
		Scan(&methodStats).Error; err != nil {
		return nil, err
	}

	return map[string]any{
		"days":         days,
		"by_major":     majorStats,
		"by_minor":     minorStats,
		"by_method":    methodStats,
		"generated_at": time.Now(),
	}, nil
}

// QueryIntentLogsByTraceID 通过 trace_id 查询该 trace 关联的所有 IntentLog
// 供 trace API 使用，由 controller 调用
func (s *IntentRecognizer) QueryIntentLogsByTraceID(ctx context.Context, traceID string)  ([]model.IntentLog, error) {
	if s.db == nil || traceID == "" {
		return nil, nil
	}
	var logs []model.IntentLog
	err := s.db.Where("trace_id = ?", traceID).Order("timestamp ASC").Find(&logs).Error
	return logs, err
}
