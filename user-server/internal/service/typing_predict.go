package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// IntentPrediction 意图预测结果
type IntentPrediction struct {
	InputHash        string    `json:"input_hash"`
	IntentType       string    `json:"intent_type"`
	IntentSubtype    string    `json:"intent_subtype"`
	Confidence       float64   `json:"confidence"`
	SuggestedReplies []string  `json:"suggested_replies"`
	Entities         []string  `json:"entities,omitempty"`
	PredictedAt      time.Time `json:"predicted_at"`
}

// TypingPredictService 打字预回复服务
//
// G15: 竞品标配功能 - 访客输入部分文字时后端实时预测意图，
// 推送 SSE 事件给前端，前端展示建议回复（不自动发送，仅推荐）。
//
// 内部机制：
//   - 输入 hash 做防抖：相同 hash 500ms 内不重复调用（避免用户快速输入导致雪崩）
//   - 预留接入点：接入已有的 intent_recognition service
type TypingPredictService struct {
	mu     sync.Mutex
	cache  map[string]*IntentPrediction
	timers map[string]*time.Timer
}

var (
	globalTypingPredict *TypingPredictService
	typingOnce          sync.Once
)

// GetTypingPredictService 获取全局单例
func GetTypingPredictService() *TypingPredictService {
	typingOnce.Do(func() {
		globalTypingPredict = &TypingPredictService{
			cache:  make(map[string]*IntentPrediction),
			timers: make(map[string]*time.Timer),
		}
	})
	return globalTypingPredict
}

// Predict 对输入文字进行意图预测
//
// 参数：
//   - text: 用户正在输入的文字（可能是不完整的片段）
//   - sessionID: 会话 ID（用于上下文增强）
//
// 返回：IntentPrediction 或 error
func (s *TypingPredictService) Predict(ctx context.Context, text, sessionID string) (*IntentPrediction, error) {
	if text == "" {
		return nil, fmt.Errorf("输入不能为空")
	}
	h := inputHash(text)

	s.mu.Lock()
	if cached, ok := s.cache[h]; ok {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	pred := s.predictText(text)

	s.mu.Lock()
	s.cache[h] = pred
	if t, exists := s.timers[h]; exists {
		t.Stop()
	}
	s.timers[h] = time.AfterFunc(10*time.Second, func() {
		s.mu.Lock()
		delete(s.cache, h)
		delete(s.timers, h)
		s.mu.Unlock()
	})
	s.mu.Unlock()

	logger.Debugf("[TypingPredict] 意图预测 intent=%s conf=%.2f replies=%d",
		pred.IntentType, pred.Confidence, len(pred.SuggestedReplies))
	return pred, nil
}

func (s *TypingPredictService) predictText(text string) *IntentPrediction {
	pred := &IntentPrediction{
		InputHash:   inputHash(text),
		PredictedAt: time.Now(),
	}
	lower := text

	switch {
	case s.containsAny(lower, []string{"多少钱", "价格", "price", "贵不贵", "便宜"}):
		pred.IntentType = "pricing"
		pred.IntentSubtype = "price_inquiry"
		pred.Confidence = 0.82
		pred.SuggestedReplies = []string{
			"这款产品目前的活动价是 ¥XXX，您看需要我发详细报价单吗？",
			"我们目前有 3 档套餐，分别对应不同的使用量，您方便告诉我月大概多少用量吗？",
		}
	case s.containsAny(lower, []string{"退款", "退", "cancel", "退钱", "不想用"}):
		pred.IntentType = "refund"
		pred.IntentSubtype = "refund_request"
		pred.Confidence = 0.88
		pred.SuggestedReplies = []string{
			"好的，退款需要您提供订单号，我这边帮您提交申请，一般 1-3 个工作日到账。",
			"请问您是要全额退款还是部分退款？方便告诉我具体原因吗？",
		}
	case s.containsAny(lower, []string{"你好", "在吗", "hi", "hello", "您好", "早"}):
		pred.IntentType = "greeting"
		pred.IntentSubtype = "hello"
		pred.Confidence = 0.95
		pred.SuggestedReplies = []string{
			"您好！我是智能助手，请问有什么可以帮到您？",
			"您好！在的，您方便简单说下遇到什么问题吗？",
		}
	case s.containsAny(lower, []string{"怎么", "如何", "步骤", "教程", "操作", "使用"}):
		pred.IntentType = "howto"
		pred.IntentSubtype = "usage_guide"
		pred.Confidence = 0.75
		pred.SuggestedReplies = []string{
			"操作步骤比较多，我先把核心 3 步发给您，您看是否清楚？",
			"我这边有一份图文教程链接，要不要发您参考？",
		}
	case s.containsAny(lower, []string{"投诉", "差评", "态度不好", "不满意", "反馈"}):
		pred.IntentType = "complaint"
		pred.IntentSubtype = "negative_feedback"
		pred.Confidence = 0.85
		pred.SuggestedReplies = []string{
			"非常抱歉给您带来不好的体验，我记录下您的问题后会转给专人跟进。",
			"您方便具体描述一下是哪方面不满意吗？我一定协助解决。",
		}
	default:
		pred.IntentType = "unknown"
		pred.IntentSubtype = "unclassified"
		pred.Confidence = 0.35
		pred.SuggestedReplies = []string{
			"您好，方便再说得具体一点吗？我好帮您分析。",
		}
	}
	return pred
}

func (s *TypingPredictService) containsAny(text string, words []string) bool {
	for _, w := range words {
		if len(w) > 0 && len(text) >= len(w) && searchInString(text, w) {
			return true
		}
	}
	return false
}

func searchInString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func inputHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// ToJSON 序列化为 JSON 字符串（用于 SSE data 字段）
func (p *IntentPrediction) ToJSON() string {
	b, _ := json.Marshal(p)
	return string(b)
}
