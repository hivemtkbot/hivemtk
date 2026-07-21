package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/dto"
	"marketing/internal/model"
)

// DialogueMemoryService 对话记忆服务（短期+长期）
type DialogueMemoryService struct {
	db         *gorm.DB
	dispatcher *llm.Dispatcher
}

const (
	shortTermWindow = 10 // 短期记忆保留最近 10 轮
	memoryTTL       = 30 * 24 * time.Hour
)

// NewDialogueMemoryService 创建对话记忆服务
func NewDialogueMemoryService(db *gorm.DB, dispatcher *llm.Dispatcher) *DialogueMemoryService {
	return &DialogueMemoryService{db: db, dispatcher: dispatcher}
}

// Message / ShortTermMemory 已迁至 dto 包（P2-6 DTO 层补全）
// 使用 dto.Message / dto.ShortTermMemory 替代本地类型

// GetOrCreateMemory 获取或创建记忆
func (s *DialogueMemoryService) GetOrCreateMemory(sessionID, customerID string) (*model.DialogueMemory, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	var mem model.DialogueMemory
	err := s.db.Where("session_id = ?", sessionID).First(&mem).Error
	if err == nil {
		return &mem, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	// 创建
	mem = model.DialogueMemory{
		SessionID:    sessionID,
		CustomerID:   customerID,
		KeyFacts:     model.JSONMap{},
		Objections:   model.JSONArray{},
		IntentTrail:  model.JSONArray{},
		SOPHistory:   model.JSONArray{},
		LastActiveAt: time.Now(),
		MessageCount: 0,
	}
	if err := s.db.Create(&mem).Error; err != nil {
		return nil, err
	}
	return &mem, nil
}

// AppendMessage 追加消息并更新记忆
func (s *DialogueMemoryService) AppendMessage(ctx context.Context, sessionID, customerID string, msg dto.Message) error {
	mem, err := s.GetOrCreateMemory(sessionID, customerID)
	if err != nil {
		return err
	}
	// 更新短期记忆
	trail := []string{}
	if mem.IntentTrail != nil {
		_ = json.Unmarshal(mustJSON(mem.IntentTrail), &trail)
	}

	// 更新 message_count
	mem.MessageCount++
	mem.LastActiveAt = time.Now()
	if msg.Role == "ai" || msg.Role == "agent" {
		mem.LastAction = truncate(msg.Content, 100)
	}

	// 每累计 5 轮触发一次长期摘要更新
	if mem.MessageCount%5 == 0 && s.dispatcher != nil {
		s.updateLongTermSummary(ctx, mem, msg)
	}

	return s.db.Save(mem).Error
}

// GetShortTermMemory 获取短期记忆（从 message_hub 取最近 N 条）
func (s *DialogueMemoryService) GetShortTermMemory(sessionID string) ([]dto.Message, error) {
	if s.db == nil {
		return nil, nil
	}
	var records []model.MessageHub
	err := s.db.Where("conversation_id = ?", sessionID).
		Order("sent_at DESC").Limit(shortTermWindow).Find(&records).Error
	if err != nil {
		return nil, err
	}
	// 转换为 Message，倒序变为正序
	msgs := make([]dto.Message, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		role := "user"
		if r.IsAIReply || r.Direction == "outbound" {
			role = "ai"
		}
		msgs = append(msgs, dto.Message{
			Role:      role,
			Content:   r.Content,
			Timestamp: r.SentAt,
		})
	}
	return msgs, nil
}

// GetLongTermMemory 获取长期记忆
func (s *DialogueMemoryService) GetLongTermMemory(sessionID string) (*model.DialogueMemory, error) {
	return s.GetOrCreateMemory(sessionID, "")
}

// UpdateKeyFacts 更新关键事实

// ListByCustomerID 根据 customerID 获取对话记忆列表
func (s *DialogueMemoryService) ListByCustomerID(customerID string, limit int) ([]*model.DialogueMemory, int64, error) {
	if s.db == nil {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 10
	}
	var mems []*model.DialogueMemory
	var total int64

	query := s.db.Model(&model.DialogueMemory{}).Where("customer_id = ?", customerID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("updated_at DESC").Limit(limit).Find(&mems).Error; err != nil {
		return nil, 0, err
	}
	return mems, total, nil
}

func (s *DialogueMemoryService) UpdateKeyFacts(sessionID string, facts map[string]string) error {
	mem, err := s.GetOrCreateMemory(sessionID, "")
	if err != nil {
		return err
	}
	existing := map[string]string{}
	if mem.KeyFacts != nil {
		_ = json.Unmarshal(mustJSON(mem.KeyFacts), &existing)
	}
	for k, v := range facts {
		existing[k] = v
	}
	data, _ := json.Marshal(existing)
	keyFacts := make(model.JSONMap)
	for k, v := range existing {
		keyFacts[k] = v
	}
	mem.KeyFacts = keyFacts
	if name, ok := facts["name"]; ok && name != "" {
		mem.CustomerName = name
	}
	if phone, ok := facts["phone"]; ok && phone != "" {
		mem.CustomerPhone = phone
	}
	if wechat, ok := facts["wechat"]; ok && wechat != "" {
		mem.CustomerWechat = wechat
	}
	if budget, ok := facts["budget"]; ok && budget != "" {
		mem.Budget = budget
	}
	if demand, ok := facts["demand"]; ok && demand != "" {
		mem.Demand = demand
	}
	_ = data
	return s.db.Save(mem).Error
}

// RecordObjection 记录异议
func (s *DialogueMemoryService) RecordObjection(sessionID, objectionType, content string) error {
	mem, err := s.GetOrCreateMemory(sessionID, "")
	if err != nil {
		return err
	}
	objs := []map[string]any{}
	if mem.Objections != nil {
		_ = json.Unmarshal(mustJSON(mem.Objections), &objs)
	}
	objs = append(objs, map[string]any{
		"type":    objectionType,
		"content": content,
		"time":    time.Now(),
	})
	mem.Objections = model.JSONArray(toIfaceSlice(objs))
	return s.db.Save(mem).Error
}

// UpdatePurchaseIntent 更新购买意向
func (s *DialogueMemoryService) UpdatePurchaseIntent(sessionID, level string) error {
	if level != "high" && level != "medium" && level != "low" {
		level = "low"
	}
	mem, err := s.GetOrCreateMemory(sessionID, "")
	if err != nil {
		return err
	}
	mem.PurchaseIntent = level
	return s.db.Save(mem).Error
}

// RecordIntent 记录意图轨迹
func (s *DialogueMemoryService) RecordIntent(sessionID, intentType string) error {
	mem, err := s.GetOrCreateMemory(sessionID, "")
	if err != nil {
		return err
	}
	trail := []string{}
	if mem.IntentTrail != nil {
		_ = json.Unmarshal(mustJSON(mem.IntentTrail), &trail)
	}
	trail = append(trail, intentType)
	if len(trail) > 30 {
		trail = trail[len(trail)-30:]
	}
	mem.IntentTrail = model.JSONArray(toIfaceSliceFromStrings(trail))
	return s.db.Save(mem).Error
}

// RecordSOP 记录 SOP 经历
func (s *DialogueMemoryService) RecordSOP(sessionID, sopName string) error {
	mem, err := s.GetOrCreateMemory(sessionID, "")
	if err != nil {
		return err
	}
	hist := []string{}
	if mem.SOPHistory != nil {
		_ = json.Unmarshal(mustJSON(mem.SOPHistory), &hist)
	}
	hist = append(hist, sopName)
	mem.SOPHistory = model.JSONArray(toIfaceSliceFromStrings(hist))
	return s.db.Save(mem).Error
}

// BuildContext 构建对话上下文（短期+长期+事实）
func (s *DialogueMemoryService) BuildContext(sessionID, customerID string) (string, error) {
	short, _ := s.GetShortTermMemory(sessionID)
	long, _ := s.GetLongTermMemory(sessionID)
	var sb strings.Builder
	sb.WriteString("【客户长期记忆】\n")
	if long != nil {
		if long.CustomerName != "" {
			sb.WriteString(fmt.Sprintf("客户姓名: %s\n", long.CustomerName))
		}
		if long.Budget != "" {
			sb.WriteString(fmt.Sprintf("预算: %s\n", long.Budget))
		}
		if long.Demand != "" {
			sb.WriteString(fmt.Sprintf("需求: %s\n", long.Demand))
		}
		if len(long.Objections) > 0 {
			sb.WriteString(fmt.Sprintf("历史异议: %v\n", long.Objections))
		}
		if long.PurchaseIntent != "" {
			sb.WriteString(fmt.Sprintf("购买意向: %s\n", long.PurchaseIntent))
		}
		if long.Summary != "" {
			sb.WriteString(fmt.Sprintf("历史摘要: %s\n", long.Summary))
		}
	}
	sb.WriteString("\n【最近对话】\n")
	for _, m := range short {
		role := "客户"
		if m.Role == "ai" || m.Role == "agent" {
			role = "我"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	return sb.String(), nil
}

// updateLongTermSummary 更新长期摘要
func (s *DialogueMemoryService) updateLongTermSummary(ctx context.Context, mem *model.DialogueMemory, lastMsg dto.Message) {
	if s.dispatcher == nil {
		return
	}
	prompt := fmt.Sprintf(`你是销冠对话分析师。请根据以下信息，生成对话的长期摘要和关键事实。

【已有摘要】: %s
【已有事实】: %v
【历史异议】: %v
【意图轨迹】: %v
【最新消息】: %s

请输出 JSON 格式:
{
  "summary": "50-150 字的对话摘要",
  "key_facts": {"key": "value"},
  "next_action_suggestion": "建议下一步动作"
}`, mem.Summary, mem.KeyFacts, mem.Objections, mem.IntentTrail, lastMsg.Content)

	result, err := s.dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioLongSummary,
		Prompt:      prompt,
		JSONMode:    true,
		MaxTokens:   500,
		Temperature: 0.3,
	})
	if err != nil {
		return
	}
	var parsed struct {
		Summary              string            `json:"summary"`
		KeyFacts             map[string]string `json:"key_facts"`
		NextActionSuggestion string            `json:"next_action_suggestion"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromStr(result.Content)), &parsed); err != nil {
		return
	}
	mem.Summary = parsed.Summary
	if len(parsed.KeyFacts) > 0 {
		mem.KeyFacts = model.JSONMap(stringMapToIface(parsed.KeyFacts))
	}
	mem.NextActionSuggestion = parsed.NextActionSuggestion
}

// 全局实例
var (
	dialogueMemoryOnce sync.Once
	dialogueMemory     *DialogueMemoryService
)

func GetDialogueMemory() *DialogueMemoryService {
	return dialogueMemory
}

func InitDialogueMemory(db *gorm.DB, dispatcher *llm.Dispatcher) *DialogueMemoryService {
	dialogueMemoryOnce.Do(func() {
		dialogueMemory = NewDialogueMemoryService(db, dispatcher)
	})
	return dialogueMemory
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func toIfaceSlice(m []map[string]any) []any {
	out := make([]any, len(m))
	for i, v := range m {
		out[i] = v
	}
	return out
}

func toIfaceSliceFromStrings(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func stringMapToIface(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
