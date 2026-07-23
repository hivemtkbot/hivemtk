package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// IntentRecognizer 销售意图识别器
type IntentRecognizer struct {
	db         *gorm.DB
	dispatcher *llm.Dispatcher
	cache      *redis.Client
	sopService *SOPService
}

// NewIntentRecognizer 创建意图识别器
func NewIntentRecognizer(db *gorm.DB, dispatcher *llm.Dispatcher, cache *redis.Client) *IntentRecognizer {
	return &IntentRecognizer{
		db:         db,
		dispatcher: dispatcher,
		cache:      cache,
	}
}

// SetSOPService 注入 SOP 服务用于意图→SOP 联动（P0-12）
func (s *IntentRecognizer) SetSOPService(ctx context.Context, svc *SOPService) {
	s.sopService = svc
}

// IntentType 销售意图类型常量
const (
	IntentPriceInquiry        = "price_inquiry"        // 询价
	IntentObjectionPrice      = "objection_price"      // 异议-价格
	IntentObjectionNeed       = "objection_need"       // 异议-不需要
	IntentObjectionTrust      = "objection_trust"      // 异议-信任
	IntentObjectionCompetitor = "objection_competitor" // 异议-竞品
	IntentObjectionTiming     = "objection_timing"     // 异议-时机
	IntentPurchase            = "purchase"             // 购买意向
	IntentAskProduct          = "ask_product"          // 咨询产品
	IntentAskService          = "ask_service"          // 咨询售后
	IntentAfterSale           = "after_sale"           // 售后问题
	IntentChurn               = "churn"                // 流失倾向
	IntentSocial              = "social"               // 社交寒暄
	IntentGreeting            = "greeting"             // 问候
	IntentComplaint           = "complaint"            // 投诉
	IntentUnknown             = "unknown"              // 未知

	// 兼容别名：sales_playbook.go 等模块使用的简写
	IntentStall         = IntentObjectionTiming
	IntentAskTrust      = IntentObjectionTrust
	IntentAskCompetitor = IntentObjectionCompetitor
)

// IntentDef 意图定义
type IntentDef struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Keywords    []string `json:"keywords"`
	Examples    []string `json:"examples"`
	Description string   `json:"description"`
}

// DefaultIntents 默认意图词典
var DefaultIntents = []IntentDef{
	{
		Type: IntentPriceInquiry, Name: "价格咨询",
		Keywords:    []string{"多少钱", "价格", "怎么卖", "怎么收费", "报价", "便宜", "优惠", "折扣", "多少钱一套", "什么价位"},
		Examples:    []string{"这个多少钱？", "你们价格怎么样？", "能不能便宜点？", "有优惠活动吗？"},
		Description: "客户询问价格或优惠",
	},
	{
		Type: IntentObjectionPrice, Name: "价格异议",
		Keywords:    []string{"太贵了", "价格高", "价格有点高", "有点高", "太贵", "不值", "不值这个价", "不划算", "太贵啦", "贵了"},
		Examples:    []string{"这个东西太贵了", "价格有点高啊", "比别家贵好多", "我感觉不值这个价"},
		Description: "客户对价格有异议",
	},
	{
		Type: IntentObjectionNeed, Name: "需求异议",
		Keywords:    []string{"不需要", "用不上", "没需求", "暂时不用", "再说吧", "再考虑", "看看吧", "再想想"},
		Examples:    []string{"我先看看吧", "暂时不需要", "我再考虑一下"},
		Description: "客户表示暂无需求",
	},
	{
		Type: IntentObjectionTrust, Name: "信任异议",
		Keywords:    []string{"不放心", "骗人", "假的", "靠谱吗", "真的吗", "不会是骗子", "信不过", "能信吗"},
		Examples:    []string{"你们是正规公司吗？", "能信吗？", "不会跑路吧？"},
		Description: "客户对品牌/服务存在信任问题",
	},
	{
		Type: IntentObjectionCompetitor, Name: "竞品异议",
		Keywords:    []string{"别的家", "别家", "竞品", "其他品牌", "友商", "对比一下", "比别家贵", "别家也是"},
		Examples:    []string{"别家比你们便宜", "我用别家用了几年了", "你们跟XX有什么区别？"},
		Description: "客户提到竞品对比",
	},
	{
		Type: IntentObjectionTiming, Name: "时机异议",
		Keywords:    []string{"再说", "过段时间", "等以后", "不急", "等几天", "最近忙", "没时间", "等下"},
		Examples:    []string{"过段时间再说吧", "最近比较忙", "等下再联系"},
		Description: "客户以时间为由推脱",
	},
	{
		Type: IntentPurchase, Name: "购买意向",
		Keywords:    []string{"怎么买", "怎么付款", "怎么下单", "下单", "购买", "要", "来一个", "买一个", "要了", "付了", "拍了", "怎么付"},
		Examples:    []string{"我要买，怎么付款？", "怎么下单？", "我直接拍了啊"},
		Description: "客户明确购买意向",
	},
	{
		Type: IntentAskProduct, Name: "产品咨询",
		Keywords:    []string{"功能", "特点", "效果", "怎么用", "如何使用", "参数", "规格", "材质", "质量", "能做什么"},
		Examples:    []string{"这个产品有什么功能？", "效果怎么样？", "怎么用？"},
		Description: "客户咨询产品细节",
	},
	{
		Type: IntentAskService, Name: "服务咨询",
		Keywords:    []string{"售后", "保修", "维修", "包邮", "多久到", "发货", "物流", "客服"},
		Examples:    []string{"售后怎么保障？", "多久能到？", "包邮吗？"},
		Description: "客户咨询售后服务",
	},
	{
		Type: IntentAfterSale, Name: "售后问题",
		Keywords:    []string{"坏了", "有问题", "故障", "用不了", "不能用了", "退货", "退换", "换货", "投诉", "退款", "怎么办", "要退货"},
		Examples:    []string{"我买的东西坏了", "想退货怎么办？", "有问题需要处理"},
		Description: "客户售后诉求",
	},
	{
		Type: IntentChurn, Name: "流失倾向",
		Keywords:    []string{"拉黑", "删除", "别发了", "别再发了", "烦", "退订", "屏蔽", "再发举报"},
		Examples:    []string{"别再发了", "我要拉黑你了", "再发我举报了"},
		Description: "客户明确表示流失/反感",
	},
	{
		Type: IntentSocial, Name: "社交寒暄",
		Keywords:    []string{"在吗", "你好", "hi", "hello", "在么", "哈喽", "在?", "你叫什么"},
		Examples:    []string{"在吗？", "你好", "hi"},
		Description: "客户社交寒暄/开场",
	},
	{
		Type: IntentComplaint, Name: "投诉",
		Keywords:    []string{"投诉", "差评", "垃圾", "气死", "愤怒", "过分", "太过分", "不负责", "解决不了"},
		Examples:    []string{"我要投诉你", "差评！", "太过分了"},
		Description: "客户投诉",
	},
}

// RecognizeResult 已迁至 dto 包（P2-6 DTO 层补全）
// 使用 dto.RecognizeResult 替代本地类型

// Recognize 识别意图
func (s *IntentRecognizer) Recognize(ctx context.Context, sessionID, customerID, text string) (*dto.RecognizeResult, error) {
	if text == "" {
		return &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0, Method: "rule"}, nil
	}

	var result *dto.RecognizeResult

	// 1. 规则匹配（O(1) 字典）
	if r := s.recognizeByRule(ctx, text); r != nil {
		s.saveRecord(ctx, sessionID, customerID, text, r, "", 0, 0)
		result = r
	} else if s.dispatcher != nil {
		// 2. LLM 识别
		if r, err := s.recognizeByLLM(ctx, text); err == nil && r != nil {
			s.saveRecord(ctx, sessionID, customerID, text, r, r.LLMModel, r.CostTokens, r.LatencyMs)
			result = r
		}
	}

	// 兜底
	if result == nil {
		result = &dto.RecognizeResult{
			IntentType:      IntentUnknown,
			IntentName:      "未知",
			Confidence:      0.3,
			ConfidenceLevel: "low",
			Sentiment:       "neutral",
			Method:          "rule",
		}
	}

	// 3. 意图→SOP 联动（P0-12）
	if customerID != "" {
		s.triggerSOPByIntent(ctx, customerID, sessionID, result.IntentType, result.Confidence)
	}

	return result, nil
}

// triggerSOPByIntent 触发匹配的 SOP
func (s *IntentRecognizer) triggerSOPByIntent(ctx context.Context, customerID, sessionID, intentType string, confidence float64) {
	if s.sopService == nil {
		return
	}
	// 仅高置信度（>= 0.7）才触发 SOP，避免误触发
	if confidence < 0.7 {
		return
	}
	matches, err := s.sopService.MatchByIntent(ctx, intentType)
	if err != nil || len(matches) == 0 {
		return
	}
	for _, agent := range matches {
		if !agent.IsActive {
			continue
		}
		// 去重：检查是否已有 running 的执行
		var count int64
		if s.db != nil {
			if err := s.db.WithContext(ctx).Model(&model.SOPExecution{}).
				Where("sop_id = ? AND customer_id = ? AND status = ?", agent.ID, customerID, SOPStatusRunning).
				Count(&count).Error; err != nil {
				continue
			}
			if count > 0 {
				continue
			}
		}
		exec, err := s.sopService.Execute(ctx, &dto.ExecuteRequest{
			SOPID:      agent.ID,
			CustomerID: customerID,
			SessionID:  sessionID,
			Input: map[string]any{
				"_trigger":     "intent",
				"_intent_type": intentType,
				"_confidence":  confidence,
			},
		})
		if err != nil {
			logger.Errorf("[Intent→SOP] 启动失败 intent=%s sop=%d: %v", intentType, agent.ID, err)
			continue
		}
		logger.Infof("[Intent→SOP] 触发成功 intent=%s sop=%d exec=%d", intentType, agent.ID, exec.ID)
	}
}

// recognizeByRule 规则匹配
func (s *IntentRecognizer) recognizeByRule(ctx context.Context, text string) *dto.RecognizeResult {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// 完全匹配优先级高
	for _, def := range DefaultIntents {
		for _, ex := range def.Examples {
			if strings.EqualFold(text, ex) {
				return &dto.RecognizeResult{
					IntentType:      def.Type,
					IntentName:      def.Name,
					Confidence:      0.95,
					ConfidenceLevel: "high",
					Sentiment:       inferSentiment(def.Type),
					Method:          "rule",
				}
			}
		}
	}
	// 关键词匹配
	var bestMatch *IntentDef
	bestScore := 0
	for _, def := range DefaultIntents {
		score := 0
		for _, kw := range def.Keywords {
			if strings.Contains(text, kw) {
				score += len(kw)
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = &def
		}
	}
	if bestMatch != nil && bestScore > 0 {
		conf := 0.7 + float64(bestScore)*0.02
		if conf > 0.92 {
			conf = 0.92
		}
		level := "medium"
		if conf >= 0.85 {
			level = "high"
		}
		return &dto.RecognizeResult{
			IntentType:      bestMatch.Type,
			IntentName:      bestMatch.Name,
			Confidence:      conf,
			ConfidenceLevel: level,
			Sentiment:       inferSentiment(bestMatch.Type),
			Method:          "rule",
		}
	}
	return nil
}

// recognizeByLLM LLM 识别
func (s *IntentRecognizer) recognizeByLLM(ctx context.Context, text string) (*dto.RecognizeResult, error) {
	intentList := make([]string, 0, len(DefaultIntents))
	for _, def := range DefaultIntents {
		intentList = append(intentList, fmt.Sprintf("%s: %s", def.Type, def.Description))
	}
	prompt := fmt.Sprintf(`你是销售对话意图识别专家。分析以下客户消息，从给定意图列表中选择最匹配的 1 个意图。

【客户消息】: %s

【意图列表】:
%s

【输出要求】(严格按 JSON 格式输出):
{
  "intent_type": "从列表中选择最匹配的 type",
  "intent_subtype": "细分类型(可填空字符串)",
  "confidence": 0.0-1.0 的置信度,
  "entities": {
    "product": "涉及的产品(可空)",
    "price": "涉及的价格(可空)",
    "quantity": "涉及的数量(可空)",
    "time": "涉及的时间(可空)"
  },
  "sentiment": "positive/negative/neutral"
}`, text, strings.Join(intentList, "\n"))

	start := time.Now()
	result, err := s.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioIntentRecognize,
		Prompt:      prompt,
		JSONMode:    true,
		MaxTokens:   500,
		Temperature: 0.2,
	})
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	var parsed struct {
		IntentType    string         `json:"intent_type"`
		IntentSubtype string         `json:"intent_subtype"`
		Confidence    float64        `json:"confidence"`
		Entities      map[string]any `json:"entities"`
		Sentiment     string         `json:"sentiment"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromStr(result.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("parse intent: %w", err)
	}
	level := "medium"
	if parsed.Confidence >= 0.85 {
		level = "high"
	} else if parsed.Confidence < 0.5 {
		level = "low"
	}
	intentName := parsed.IntentType
	for _, def := range DefaultIntents {
		if def.Type == parsed.IntentType {
			intentName = def.Name
			break
		}
	}
	return &dto.RecognizeResult{
		IntentType:      parsed.IntentType,
		IntentName:      intentName,
		Confidence:      parsed.Confidence,
		ConfidenceLevel: level,
		IntentSubtype:   parsed.IntentSubtype,
		Entities:        parsed.Entities,
		Sentiment:       parsed.Sentiment,
		Method:          "llm",
		LLMModel:        result.Model,
		CostTokens:      result.TotalTokens,
		LatencyMs:       latency,
	}, nil
}

// inferSentiment 根据意图推断情感
func inferSentiment(intentType string) string {
	switch intentType {
	case IntentChurn, IntentComplaint, IntentObjectionPrice, IntentObjectionNeed, IntentObjectionTrust:
		return "negative"
	case IntentPurchase, IntentPriceInquiry:
		return "positive"
	default:
		return "neutral"
	}
}

// saveRecord 保存识别记录
func (s *IntentRecognizer) saveRecord(ctx context.Context, sessionID, customerID, text string, result *dto.RecognizeResult, llmModel string, costTokens, latencyMs int) {
	if s.db == nil {
		return
	}
	entitiesJSON, _ := json.Marshal(result.Entities)
	rec := &model.IntentRecord{
		SessionID:       sessionID,
		CustomerID:      customerID,
		RawText:         text,
		IntentType:      result.IntentType,
		IntentSubtype:   result.IntentSubtype,
		Confidence:      result.Confidence,
		ConfidenceLevel: result.ConfidenceLevel,
		Entities:        model.JSONMap(entitiesToMap(entitiesJSON)),
		Sentiment:       result.Sentiment,
		LLMModel:        llmModel,
		CostTokens:      costTokens,
		LatencyMs:       latencyMs,
	}
	// R6 修复：原 fire-and-forget goroutine 无 recover、无 ctx，panic 会杀进程，shutdown 无法取消。
	// 改为：使用 context.Background()（异步落库不依赖请求生命周期）+ recover 防 panic + 错误日志。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("intent_recognition: async persist recovered from panic: %v", r)
			}
		}()
		if err := s.db.Create(rec).Error; err != nil {
			logger.Errorf("intent_recognition: async persist intent record failed: %v", err)
		}
	}()
}

func entitiesToMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func extractJSONFromStr(s string) string {
	s = strings.TrimSpace(s)
	// 寻找第一个 { 和最后一个 }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// GetIntentStats 获取意图统计
func (s *IntentRecognizer) GetIntentStats(ctx context.Context, days int) (map[string]int, error) {
	if s.db == nil {
		return map[string]int{}, nil
	}
	since := time.Now().AddDate(0, 0, -days)
	var records []model.IntentRecord
	err := s.db.Where("created_at > ?", since).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	stats := make(map[string]int)
	for _, r := range records {
		stats[r.IntentType]++
	}
	return stats, nil
}

// GetRecentIntents 客户近期意图历史
func (s *IntentRecognizer) GetRecentIntents(ctx context.Context, customerID string, limit int) ([]model.IntentRecord, error) {
	if s.db == nil {
		return nil, nil
	}
	var records []model.IntentRecord
	err := s.db.Where("customer_id = ?", customerID).
		Order("created_at DESC").Limit(limit).Find(&records).Error
	return records, err
}

// 全局实例管理
var (
	intentRecognizerOnce sync.Once
	intentRecognizer     *IntentRecognizer
)

// GetIntentRecognizer 获取意图识别器单例
func GetIntentRecognizer() *IntentRecognizer {
	return intentRecognizer
}

// InitIntentRecognizer 初始化意图识别器
func InitIntentRecognizer(db *gorm.DB, dispatcher *llm.Dispatcher, cache *redis.Client) *IntentRecognizer {
	intentRecognizerOnce.Do(func() {
		intentRecognizer = NewIntentRecognizer(db, dispatcher, cache)
	})
	return intentRecognizer
}
