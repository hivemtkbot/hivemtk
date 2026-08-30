package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

const (
	leadMiningQueueSize       = 8192
	leadMiningWorkers         = 4
	leadMiningDebounce        = 60 * time.Second 
	leadMiningHistorySize     = 20               
	leadMiningConfigTTL       = 30 * time.Second
	leadMiningHighOpportunity = 70 
)

// LeadJudgement LLM 结构化判定结果
type LeadJudgement struct {
	IsLead          bool     `json:"is_lead"`
	IntentScore     int      `json:"intent_score"`     
	MatchedKeywords []string `json:"matched_keywords"` 
	MatchedTags     []string `json:"matched_tags"`     
	Summary         string   `json:"summary"`          
	Confidence      float64  `json:"confidence"`       
	Reason          string   `json:"reason"`           
}

// LLMJudge 判定接口（便于测试注入 fake）
type LLMJudge interface {
	Judge(ctx context.Context, cfg *model.LeadMiningConfig, history []llm.ChatMessage) (*LeadJudgement, error)
}

// Service 线索发掘服务（非侵入异步）
type Service struct {
	queue          chan *model.MessageHub
	workers        int
	judge          LLMJudge
	historyFetcher func(ctx context.Context, hub *model.MessageHub) []llm.ChatMessage
	custRepo       repository.CustomerRepository
	clueRepo       repository.ClueRepository
	cfgRepo        repository.LeadMiningConfigRepository
	hubRepo        *repository.MessageHubRepository
	mu             sync.Mutex
	lastJudge      map[string]time.Time
	cfgCache       *model.LeadMiningConfig
	cfgLoadedAt    time.Time
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// singleton 全局单例，供控制器热更新配置缓存
var singleton *Service

// NewLeadMiningService 构造并启动线索发掘服务（后台 worker，非阻塞）
func NewLeadMiningService() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		queue:     make(chan *model.MessageHub, leadMiningQueueSize),
		workers:   leadMiningWorkers,
		judge:     &dispatcherJudge{},
		custRepo:  repository.NewCustomerRepository(),
		clueRepo:  repository.NewClueRepository(),
		cfgRepo:   repository.NewLeadMiningConfigRepository(),
		hubRepo:   repository.NewMessageHubRepository(),
		lastJudge: make(map[string]time.Time),
		cancel:    cancel,
	}
	singleton = s
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}
	logger.Infof("[lead-mining] 服务已启动，worker=%d queue=%d", s.workers, leadMiningQueueSize)
	return s
}

// ReloadConfigCache 热更新配置缓存（控制器保存配置后调用）
func ReloadConfigCache() {
	if singleton == nil {
		return
	}
	singleton.mu.Lock()
	singleton.cfgCache = nil
	singleton.mu.Unlock()
}

// GetLeadMiningConfig 读取线索发掘全局配置（controller 层入口，避免 controller 直连 repository/model）
func GetLeadMiningConfig(ctx context.Context) (*dto.LeadMiningConfig, error) {
	cfg, err := repository.NewLeadMiningConfigRepository().GetSingleton(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &dto.LeadMiningConfig{}, nil
	}
	return leadMiningConfigToDTO(cfg), nil
}

// SaveLeadMiningConfig 保存配置并热更新运行中的缓存


// GetStatus 返回线索挖掘服务运行时状态
func GetStatus() map[string]any {
    s := singleton
    if s == nil {
        return map[string]any{"running": false, "message": "service not initialized"}
    }
    s.mu.Lock()
    count := len(s.lastJudge)
    s.mu.Unlock()
    return map[string]any{
        "running":    true,
        "workers":    s.workers,
        "queue_depth": len(s.queue),
        "recent_processed": count,
        "cfg_loaded":       s.cfgLoadedAt.IsZero(),
    }
}

func SaveLeadMiningConfig(ctx context.Context, in *dto.LeadMiningConfig) error {
	if err := repository.NewLeadMiningConfigRepository().Save(ctx, leadMiningConfigFromDTO(in)); err != nil {
		return err
	}
	ReloadConfigCache()
	return nil
}

func leadMiningConfigToDTO(m *model.LeadMiningConfig) *dto.LeadMiningConfig {
	return &dto.LeadMiningConfig{
		ID:             m.ID,
		Enabled:        m.Enabled,
		Keywords:       []string(m.Keywords),
		Tags:           []string(m.Tags),
		Requirement:    m.Requirement,
		Channels:       []string(m.Channels),
		MinIntentScore: m.MinIntentScore,
		Model:          m.Model,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func leadMiningConfigFromDTO(d *dto.LeadMiningConfig) *model.LeadMiningConfig {
	return &model.LeadMiningConfig{
		ID:             d.ID,
		Enabled:        d.Enabled,
		Keywords:       model.JSONStrings(d.Keywords),
		Tags:           model.JSONStrings(d.Tags),
		Requirement:    d.Requirement,
		Channels:       model.JSONStrings(d.Channels),
		MinIntentScore: d.MinIntentScore,
		Model:          d.Model,
	}
}

// Stop 停止服务（进程退出时）
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// Enqueue 非阻塞入队：核心业务只在消息落库后调用一次，绝不阻塞/入侵
func (s *Service) Enqueue(hub *model.MessageHub) {
	if s == nil || hub == nil {
		return
	}
	select {
	case s.queue <- hub:
	default:
		logger.Warnf("[lead-mining] 队列已满，丢弃消息 msgID=%s", hub.MsgID)
	}
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case hub := <-s.queue:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[lead-mining] worker panic recovered: %v", r)
					}
				}()
				s.process(ctx, hub)
			}()
		}
	}
}

// process 单条消息的异步判定流程（在 worker 内执行，与核心业务解耦）
func (s *Service) process(ctx context.Context, hub *model.MessageHub) {
	if hub == nil || hub.Direction != "inbound" {
		return
	}
	content := strings.TrimSpace(hub.Content)
	if content == "" {
		return
	}

	cfg := s.loadConfig(ctx)
	if cfg == nil || !cfg.Enabled {
		return
	}
	if !channelEnabled(cfg, hub.Platform) {
		return
	}

	// debounceKey: 群聊用 platform:convID:senderID，单聊用 platform:senderID
	// 同一用户在不同群聊中的行为应独立判定
	account := fmt.Sprintf("%s:%s", hub.Platform, hub.SenderID)
	debounceKey := account
	if hub.IsGroup && hub.ConversationID != "" {
		debounceKey = fmt.Sprintf("%s:%s:%s", hub.Platform, hub.ConversationID, hub.SenderID)
	}
	s.mu.Lock()
	if last, ok := s.lastJudge[debounceKey]; ok && time.Since(last) < leadMiningDebounce {
		s.mu.Unlock()
		return
	}
	s.lastJudge[debounceKey] = time.Now()
	s.mu.Unlock()

	history := s.fetchHistory(ctx, hub)
	if len(history) == 0 {
		return
	}

	jd, err := s.judge.Judge(ctx, cfg, history)
	if err != nil {
		logger.Warnf("[lead-mining] 判定失败 account=%s: %v", account, err)
		return
	}
	if !jd.IsLead || jd.IntentScore < cfg.MinIntentScore {
		return
	}

	s.persistLead(ctx, cfg, hub, account, jd)
}

// loadConfig 读取配置（带 TTL 缓存）
func (s *Service) loadConfig(ctx context.Context) *model.LeadMiningConfig {
	s.mu.Lock()
	if s.cfgCache != nil && time.Since(s.cfgLoadedAt) < leadMiningConfigTTL {
		c := s.cfgCache
		s.mu.Unlock()
		return c
	}
	s.mu.Unlock()

	cfg, err := s.cfgRepo.GetSingleton(ctx)
	if err != nil {
		logger.Warnf("[lead-mining] 读取配置失败: %v", err)
		return nil
	}
	s.mu.Lock()
	s.cfgCache = cfg
	s.cfgLoadedAt = time.Now()
	s.mu.Unlock()
	return cfg
}

// fetchHistory 取该客户的多轮会话历史（可注入 fetcher 便于测试，默认走 DB）
func (s *Service) fetchHistory(ctx context.Context, hub *model.MessageHub) []llm.ChatMessage {
	if s.historyFetcher != nil {
		return s.historyFetcher(ctx, hub)
	}
	return s.fetchHistoryDB(ctx, hub)
}

// fetchHistoryDB 从 message_hub 取该客户最近 N 条入站消息，按时间正序构造多轮上下文
func (s *Service) fetchHistoryDB(ctx context.Context, hub *model.MessageHub) []llm.ChatMessage {
	hubs, err := s.hubRepo.ListRecentInboundBySender(ctx, hub.Platform, hub.SenderID, leadMiningHistorySize)
	if err != nil {
		logger.Warnf("[lead-mining] 读取会话历史失败: %v", err)
		return nil
	}
	for i, j := 0, len(hubs)-1; i < j; i, j = i+1, j-1 {
		hubs[i], hubs[j] = hubs[j], hubs[i]
	}
	msgs := make([]llm.ChatMessage, 0, len(hubs))
	for _, h := range hubs {
		c := strings.TrimSpace(h.Content)
		if c == "" {
			continue
		}
		msgs = append(msgs, llm.ChatMessage{Role: "user", Content: c})
	}
	return msgs
}

// persistLead 打标签 + 写入线索库存
func (s *Service) persistLead(ctx context.Context, cfg *model.LeadMiningConfig, hub *model.MessageHub, account string, jd *LeadJudgement) {
	customer, err := s.resolveCustomer(ctx, hub.Platform, hub.SenderID, hub.SenderName)
	if err != nil || customer == nil {
		logger.Warnf("[lead-mining] 解析客户失败 account=%s: %v", account, err)
		return
	}

	tags := mergeTags(cfg.Tags, jd.MatchedTags)
	if len(tags) > 0 {
		if err := s.tagCustomer(ctx, customer, tags); err != nil {
			logger.Warnf("[lead-mining] 打标签失败 customer=%s: %v", customer.ID, err)
		}
	}

	// 通用 LLM 线索发掘路径统一使用 ClueTypeLeadMining 类型（type=8），
	// 与渠道专用关键词 miner（如 ClueTypeTelegram/ClueTypeDouyin）区分开，
	// 避免两套路径写同一 (type, account) 线索时意向分来源混乱。
	clueType := ClueTypeLeadMining

	clue := &model.Clue{
		Type:           clueType,
		Account:        account,
		Name:           displayName(customer, hub),
		Desc:           jd.Summary,
		IntentScore:    int64(jd.IntentScore),
		IsOpportunity:  boolToInt(jd.IntentScore >= leadMiningHighOpportunity),
		ConversationID: hub.ConversationID,
		OneID:          customer.UnifiedID,
		SourceID:       "lead_mining",
		MessageID:      fmt.Sprintf("%d", hub.ID),
		OwnerAccount:   hub.AccountID,
		IsGroup:        hub.IsGroup,
		GroupID:        hub.GroupID,
	}

	existing, gerr := s.clueRepo.FindByTypeAndAccount(ctx, clueType, account)
	if gerr == nil && existing != nil {
		_ = s.clueRepo.UpdateByID(ctx, existing.ID, map[string]any{
			"intent_score":    int64(jd.IntentScore),
			"desc":            jd.Summary,
			"is_opportunity":  boolToInt(jd.IntentScore >= leadMiningHighOpportunity),
			"conversation_id": hub.ConversationID,
			"message_id":      fmt.Sprintf("%d", hub.ID),
			"is_group":        hub.IsGroup,
			"group_id":        hub.GroupID,
		})
		return
	}
	if err := s.clueRepo.Create(ctx, clue); err != nil {
		logger.Warnf("[lead-mining] 写入线索失败 account=%s: %v", account, err)
	}
}

// resolveCustomer 按稳定键 lm:platform:sender 复用/创建客户（避免重复建客户）
func (s *Service) resolveCustomer(ctx context.Context, platform, senderID, name string) (*model.Customer, error) {
	if platform == "" || senderID == "" {
		return nil, fmt.Errorf("无客户身份标识")
	}
	key := "lm:" + platform + ":" + senderID
	if c, gerr := s.custRepo.GetByUnifiedID(ctx, key); gerr == nil && c != nil {
		// 仅当 name 有值且客户名称为空/占位时才补充更新
		if c.Name == "" || c.Name == key {
			if name != "" {
				c.Name = name
				_ = s.custRepo.Update(ctx, c)
			}
		}
		return c, nil
	}
	c := &model.Customer{UnifiedID: key, Name: name}
	if cerr := s.custRepo.Create(ctx, c); cerr != nil {
		if c2, g2 := s.custRepo.GetByUnifiedID(ctx, key); g2 == nil && c2 != nil {
			return c2, nil
		}
		return nil, cerr
	}
	return c, nil
}

// tagCustomer 合并标签并写入 Customer.Tags（JSON 数组）
func (s *Service) tagCustomer(ctx context.Context, customer *model.Customer, newTags []string) error {
	var cur []string
	_ = json.Unmarshal([]byte(customer.Tags), &cur)
	merged := mergeTags(cur, newTags)
	b, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	customer.Tags = string(b)
	return s.custRepo.Update(ctx, customer)
}


func channelEnabled(cfg *model.LeadMiningConfig, platform string) bool {
	if len(cfg.Channels) == 0 {
		return true
	}
	for _, c := range cfg.Channels {
		if strings.EqualFold(strings.TrimSpace(c), strings.TrimSpace(platform)) {
			return true
		}
	}
	return false
}

// mergeTags 合并两组标签，按小写去重（避免 AI兴趣/ai兴趣 重复），保留首次出现的原始大小写
func mergeTags(a, b []string) []string {
	set := map[string]string{} 
	add := func(t string) {
		if v := strings.TrimSpace(t); v != "" {
			k := strings.ToLower(v)
			if _, ok := set[k]; !ok {
				set[k] = v
			}
		}
	}
	for _, t := range a {
		add(t)
	}
	for _, t := range b {
		add(t)
	}
	out := make([]string, 0, len(set))
	for _, v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func displayName(c *model.Customer, hub *model.MessageHub) string {
	if hub != nil && hub.SenderName != "" {
		return hub.SenderName
	}
	if c != nil && c.ID != "" {
		return c.ID
	}
	if hub != nil {
		return hub.SenderID
	}
	return "未知"
}

// dispatcherJudge 基于全局 LLM dispatcher 的默认判定实现
type dispatcherJudge struct{}

func (j *dispatcherJudge) Judge(ctx context.Context, cfg *model.LeadMiningConfig, history []llm.ChatMessage) (*LeadJudgement, error) {
	d := llm.GetGlobalDispatcher()
	if d == nil {
		return nil, fmt.Errorf("llm dispatcher 未初始化")
	}
	req := llm.DispatchRequest{
		Scenario:     llm.ScenarioLowCost,
		SystemPrompt: buildSystemPrompt(cfg),
		Messages:     history,
		MaxTokens:    1024,
		Temperature:  0.2,
	}
	var jd LeadJudgement
	if _, err := d.DispatchStructured(ctx, req, &jd); err != nil {
		return nil, err
	}
	return &jd, nil
}

// buildSystemPrompt 构造系统提示词（含用户配置的关键词/标签/要求）
func buildSystemPrompt(cfg *model.LeadMiningConfig) string {
	var b strings.Builder
	b.WriteString("你是企业私域的「线索发掘」助手。下面是运营设置的发掘规则，请你基于提供的多轮客户对话，判断是否构成值得跟进的销售线索。\n\n")
	if len(cfg.Keywords) > 0 {
		b.WriteString("【关键词】命中以下任一关键词应加分（仅参考，不是唯一标准）：" + strings.Join(cfg.Keywords, "、") + "\n")
	}
	if len(cfg.Tags) > 0 {
		b.WriteString("【期望标签】命中线索后应给客户打的标签：" + strings.Join(cfg.Tags, "、") + "\n")
	}
	if strings.TrimSpace(cfg.Requirement) != "" {
		b.WriteString("【线索要求】运营对「什么是线索」的明确要求：\n" + cfg.Requirement + "\n")
	}
	b.WriteString("\n请综合全部对话，输出严格 JSON：\n")
	b.WriteString("{\n  \"is_lead\": bool,\n  \"intent_score\": 0-100 的整数（>=70 强意向，40-69 中等，<40 弱）,\n")
	b.WriteString("  \"matched_keywords\": [命中的关键词],\n  \"matched_tags\": [建议追加的标签],\n")
	b.WriteString("  \"summary\": \"一句话线索摘要（含客户诉求/场景/预算等要点）\",\n  \"confidence\": 0-1 的置信度,\n  \"reason\": \"判定理由\"\n}\n")
	b.WriteString("只输出 JSON，不要额外解释。")
	return b.String()
}

