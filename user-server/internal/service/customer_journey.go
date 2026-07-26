package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"marketing/internal/cache"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 商业产品级 客户旅程状态机（Customer Journey State Machine）
// ----------------------------------------------------------------------------
// 商业市场需求：每个客户都处于旅程的某个阶段，AI/销售的所有动作都应该
// 与当前阶段匹配。例如"陌生"客户发优惠券 = 浪费；"成交"客户被再次销售
// = 骚扰。Salesforce、HubSpot 都以客户旅程为核心。
//
// 完整 9 阶段：陌生 → 留资 → 初步接触 → 意向 → 报价 → 成交 → 售后 → 复购 → 沉睡
//
// 持久化（CC-P2 修复）：状态变更同步写 Redis（key=journey:state:{customer_id}），
// 内存 map 作为 L1 缓存，Redis 作为 source of truth，跨实例共享。Redis
// 不可用时优雅降级为纯内存模式，仅影响持久性不影响功能。
// ============================================================================

// Redis 持久化相关常量
const (
	journeyStateKeyPrefix = "journey:state:"

	// 阶段级 TTL：活跃阶段 30 天，事务阶段 90 天，沉睡 180 天，已流失 365 天
	journeyTTLAcquisition   = 30 * 24 * time.Hour
	journeyTTLTransactional = 90 * 24 * time.Hour
	journeyTTLSleeping      = 180 * 24 * time.Hour
	journeyTTLLost          = 365 * 24 * time.Hour
	journeyTTLDefault       = 30 * 24 * time.Hour
)

// journeyStateKey 拼接 Redis key
func journeyStateKey(customerID string) string {
	return journeyStateKeyPrefix + customerID
}

// journeyStateTTL 根据当前阶段返回动态 TTL
func journeyStateTTL(stage JourneyStage) time.Duration {
	switch stage {
	case StageStranger, StageLead, StageContact, StageInterested:
		return journeyTTLAcquisition
	case StageQuoted, StageWon, StageAfterSale, StageRepurchase:
		return journeyTTLTransactional
	case StageSleeping:
		return journeyTTLSleeping
	case StageLost:
		return journeyTTLLost
	default:
		return journeyTTLDefault
	}
}

// cloneJourneyState 深拷贝，用于在锁外安全地序列化。
func cloneJourneyState(src *JourneyState) JourneyState {
	if src == nil {
		return JourneyState{}
	}
	dst := *src
	if src.StageHistory != nil {
		dst.StageHistory = append([]JourneyEvent(nil), src.StageHistory...)
	}
	if src.AutoTags != nil {
		dst.AutoTags = append([]string(nil), src.AutoTags...)
	}
	if src.Metadata != nil {
		dst.Metadata = make(map[string]string, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}
	return dst
}

// JourneyStage 客户旅程阶段
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type JourneyStage = dto.JourneyStage

// StageXxx 常量别名（与 dto.StageXxx 一致）
const (
	StageStranger   = dto.StageStranger
	StageLead       = dto.StageLead
	StageContact    = dto.StageContact
	StageInterested = dto.StageInterested
	StageQuoted     = dto.StageQuoted
	StageWon        = dto.StageWon
	StageAfterSale  = dto.StageAfterSale
	StageRepurchase = dto.StageRepurchase
	StageSleeping   = dto.StageSleeping
	StageLost       = dto.StageLost
)

// AllStages 所有阶段
// 已迁移至 dto 包，此处保留变量别名以维持向后兼容
var AllStages = dto.AllStages

// StageMeta 阶段元信息
type StageMeta struct {
	Stage           JourneyStage  `json:"stage"`
	Label           string        `json:"label"`
	Description     string        `json:"description"`
	DefaultFollowup time.Duration `json:"default_followup"`          // 默认跟进间隔
	RecommendedSOP  string        `json:"recommended_sop"`           // 推荐 SOP
	OwnerRole       string        `json:"owner_role"`                // 负责角色
	AllowAIHandle   bool          `json:"allow_ai_handle"`           // 是否允许 AI 接管
	AutoNextStage   JourneyStage  `json:"auto_next_stage,omitempty"` // 自动迁移下一阶段
}

// StageMetas 阶段配置
var StageMetas = map[JourneyStage]*StageMeta{
	StageStranger: {
		Stage: StageStranger, Label: "陌生客户", Description: "首次接触，未留资",
		DefaultFollowup: 0, RecommendedSOP: "welcome_greeting", OwnerRole: "ai",
		AllowAIHandle: true,
	},
	StageLead: {
		Stage: StageLead, Label: "已留资", Description: "留下联系方式，待分配",
		DefaultFollowup: 30 * time.Minute, RecommendedSOP: "first_contact", OwnerRole: "ai",
		AllowAIHandle: true,
	},
	StageContact: {
		Stage: StageContact, Label: "已加微", Description: "添加企微/微信成功",
		DefaultFollowup: 24 * time.Hour, RecommendedSOP: "value_proposition", OwnerRole: "ai",
		AllowAIHandle: true,
	},
	StageInterested: {
		Stage: StageInterested, Label: "有意向", Description: "主动咨询产品/价格",
		DefaultFollowup: 4 * time.Hour, RecommendedSOP: "product_intro", OwnerRole: "ai+sales",
		AllowAIHandle: true,
	},
	StageQuoted: {
		Stage: StageQuoted, Label: "已报价", Description: "已发送价格/方案，等待决策",
		DefaultFollowup: 24 * time.Hour, RecommendedSOP: "objection_handling", OwnerRole: "sales",
		AllowAIHandle: true, AutoNextStage: StageInterested,
	},
	StageWon: {
		Stage: StageWon, Label: "已成交", Description: "已下单/已付款",
		DefaultFollowup: 0, RecommendedSOP: "thank_you", OwnerRole: "sales",
		AllowAIHandle: true, AutoNextStage: StageAfterSale,
	},
	StageAfterSale: {
		Stage: StageAfterSale, Label: "售后中", Description: "服务履约/已交付",
		DefaultFollowup: 7 * 24 * time.Hour, RecommendedSOP: "after_sale_care", OwnerRole: "cs",
		AllowAIHandle: true, AutoNextStage: StageRepurchase,
	},
	StageRepurchase: {
		Stage: StageRepurchase, Label: "复购期", Description: "服务完成后 30 天内",
		DefaultFollowup: 15 * 24 * time.Hour, RecommendedSOP: "repurchase_reminder", OwnerRole: "ai",
		AllowAIHandle: true, AutoNextStage: StageSleeping,
	},
	StageSleeping: {
		Stage: StageSleeping, Label: "沉睡", Description: "30-90 天无互动",
		DefaultFollowup: 7 * 24 * time.Hour, RecommendedSOP: "reactivation", OwnerRole: "ai",
		AllowAIHandle: true, AutoNextStage: StageLost,
	},
	StageLost: {
		Stage: StageLost, Label: "已流失", Description: "明确拒绝/拉黑/90天以上未互动",
		DefaultFollowup: 0, RecommendedSOP: "win_back", OwnerRole: "sales",
		AllowAIHandle: false,
	},
}

// JourneyEvent 旅程事件
type JourneyEvent struct {
	Type       string         `json:"type"`        // 事件类型
	FromStage  JourneyStage   `json:"from_stage"`  // 起始阶段
	ToStage    JourneyStage   `json:"to_stage"`    // 目标阶段
	Reason     string         `json:"reason"`      // 原因
	Source     string         `json:"source"`      // 来源：ai_chat / order / manual / system
	OperatorID string         `json:"operator_id"` // 操作者（AI / 销售 / 系统）
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// JourneyState 旅程状态
type JourneyState struct {
	CustomerID   string            `json:"customer_id"`
	OneID        string            `json:"one_id"`
	CurrentStage JourneyStage      `json:"current_stage"`
	StageSince   time.Time         `json:"stage_since"`   // 进入当前阶段时间
	StageHistory []JourneyEvent    `json:"stage_history"` // 阶段迁移历史
	LastTouchAt  time.Time         `json:"last_touch_at"` // 最后互动时间
	TotalTouches int               `json:"total_touches"` // 总互动次数
	AutoTags     []string          `json:"auto_tags"`     // 自动标签
	Metadata     map[string]string `json:"metadata"`
}

// CustomerJourneyService 客户旅程服务
type CustomerJourneyService struct {
	mu          sync.RWMutex
	states      map[string]*JourneyState // customerID → state（L1 缓存）
	cache       cache.Cache              // Redis 持久化后端（可降级为内存）
	subscribers []JourneySubscriber
}

// JourneySubscriber 旅程订阅者
type JourneySubscriber interface {
	OnJourneyEvent(ctx context.Context, customerID string, event *JourneyEvent)
}

// NewCustomerJourneyService 创建客户旅程服务（默认注入全局缓存后端）
func NewCustomerJourneyService() *CustomerJourneyService {
	return &CustomerJourneyService{
		states: make(map[string]*JourneyState),
		cache:  cache.GetGlobalCache(),
	}
}

// NewCustomerJourneyServiceWithCache 使用指定缓存后端创建服务（依赖注入用）
func NewCustomerJourneyServiceWithCache(c cache.Cache) *CustomerJourneyService {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	return &CustomerJourneyService{
		states: make(map[string]*JourneyState),
		cache:  c,
	}
}

// SetCache 注入/替换缓存后端（main 装配或测试场景）
func (s *CustomerJourneyService) SetCache(c cache.Cache) {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	s.mu.Lock()
	s.cache = c
	s.mu.Unlock()
}

// persistState 写 Redis（最佳努力，失败仅记录日志，不影响主流程）
func (s *CustomerJourneyService) persistState(ctx context.Context, state *JourneyState) {
	if state == nil || state.CustomerID == "" {
		return
	}
	if s.cache == nil {
		return
	}
	ttl := journeyStateTTL(state.CurrentStage)
	if err := s.cache.SetJSON(ctx, journeyStateKey(state.CustomerID), state, ttl); err != nil {
		logger.Warnf("journey.persistState(%s) stage=%s error: %v",
			state.CustomerID, state.CurrentStage, err)
	}
}

// GetState 获取客户旅程状态（L1 内存 → L2 Redis → 默认陌生客户）
func (s *CustomerJourneyService) GetState(ctx context.Context, customerID string) *JourneyState {
	// L1：进程内缓存（毫秒级）
	s.mu.RLock()
	if state, ok := s.states[customerID]; ok {
		s.mu.RUnlock()
		c := *state
		return &c
	}
	s.mu.RUnlock()

	// L2：Redis 持久化（跨实例共享，弥补重启丢失）
	if s.cache != nil {
		var loaded JourneyState
		if err := s.cache.GetJSON(ctx, journeyStateKey(customerID), &loaded); err == nil && loaded.CustomerID != "" {
			// 回填 L1（写时复制，避免指针共享被外部修改）
			s.mu.Lock()
			stored := loaded
			s.states[customerID] = &stored
			s.mu.Unlock()
			out := loaded
			return &out
		}
		// 错误或空 → 视为未命中，继续走默认分支（Redis 不可用时静默降级）
	}

	// 默认陌生客户
	return &JourneyState{
		CustomerID:   customerID,
		CurrentStage: StageStranger,
		StageSince:   time.Now(),
		StageHistory: []JourneyEvent{},
		TotalTouches: 0,
		AutoTags:     []string{},
		Metadata:     make(map[string]string),
	}
}

// Touch 记录互动（不改变阶段）
func (s *CustomerJourneyService) Touch(ctx context.Context, customerID, source string) {
	s.mu.Lock()
	state, ok := s.states[customerID]
	if !ok {
		state = &JourneyState{
			CustomerID:   customerID,
			CurrentStage: StageContact,
			StageSince:   time.Now(),
			StageHistory: []JourneyEvent{},
			AutoTags:     []string{},
			Metadata:     make(map[string]string),
		}
		s.states[customerID] = state
	}
	state.LastTouchAt = time.Now()
	state.TotalTouches++
	snapshot := cloneJourneyState(state)
	s.mu.Unlock()
	// 锁外异步持久化（深拷贝保证安全；Redis 失败仅记录日志）
	s.persistState(ctx, &snapshot)
}

// Transition 迁移阶段
func (s *CustomerJourneyService) Transition(ctx context.Context, customerID string, toStage JourneyStage, source, operatorID, reason string, metadata map[string]any) (*JourneyEvent, error) {
	if !s.isValidStage(ctx, toStage) {
		return nil, fmt.Errorf("无效的阶段: %s", toStage)
	}
	s.mu.Lock()
	state, ok := s.states[customerID]
	if !ok {
		state = &JourneyState{
			CustomerID:   customerID,
			CurrentStage: StageStranger,
			StageSince:   time.Now(),
			StageHistory: []JourneyEvent{},
			AutoTags:     []string{},
			Metadata:     make(map[string]string),
		}
		s.states[customerID] = state
	}
	fromStage := state.CurrentStage
	if fromStage == toStage {
		// 同阶段不迁移
		s.mu.Unlock()
		return nil, nil
	}
	// 阶段迁移
	event := &JourneyEvent{
		Type:       "stage_transition",
		FromStage:  fromStage,
		ToStage:    toStage,
		Reason:     reason,
		Source:     source,
		OperatorID: operatorID,
		Metadata:   metadata,
		Timestamp:  time.Now(),
	}
	state.CurrentStage = toStage
	state.StageSince = time.Now()
	state.StageHistory = append(state.StageHistory, *event)
	// 触发的副作用
	s.applyStageSideEffects(ctx, state, toStage)
	snapshot := cloneJourneyState(state)
	s.mu.Unlock()
	// 锁外持久化到 Redis（深拷贝安全；失败仅降级不影响主流程）
	s.persistState(ctx, &snapshot)
	// 通知订阅者
	for _, sub := range s.subscribers {
		go sub.OnJourneyEvent(ctx, customerID, event)
	}
	return event, nil
}

// applyStageSideEffects 阶段副作用（打标签 + 推荐 SOP）
func (s *CustomerJourneyService) applyStageSideEffects(ctx context.Context, state *JourneyState, stage JourneyStage) {
	meta := StageMetas[stage]
	if meta == nil {
		return
	}
	// 自动打标签
	tag := "stage:" + string(stage)
	hasTag := false
	for _, t := range state.AutoTags {
		if t == tag {
			hasTag = true
			break
		}
	}
	if !hasTag {
		state.AutoTags = append(state.AutoTags, tag)
	}
}

// isValidStage 检查阶段有效性
func (s *CustomerJourneyService) isValidStage(ctx context.Context, stage JourneyStage) bool {
	for _, s := range AllStages {
		if s == stage {
			return true
		}
	}
	return false
}

// AddSubscriber 添加订阅者
func (s *CustomerJourneyService) AddSubscriber(ctx context.Context, sub JourneySubscriber) {
	s.subscribers = append(s.subscribers, sub)
}

// ListByStage 按阶段列出客户
func (s *CustomerJourneyService) ListByStage(ctx context.Context, stage JourneyStage) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	customerIDs := []string{}
	for cid, state := range s.states {
		if state.CurrentStage == stage {
			customerIDs = append(customerIDs, cid)
		}
	}
	return customerIDs
}

// AutoDetectSleeping 自动检测沉睡客户
// 商业逻辑：成交后 30 天未互动 → 复购期；复购期 60 天未互动 → 沉睡
// DefaultFollowup=0 的阶段（如 won）也需处理，否则成交客户无法沉淀
// 商业产品级要求：所有"持续型"阶段（contact → repurchase）必须可检测沉睡
func (s *CustomerJourneyService) AutoDetectSleeping(ctx context.Context) []string {
	s.mu.Lock()
	wokeUp := []string{}
	now := time.Now()
	snapshots := make(map[string]JourneyState, 0)
	for cid, state := range s.states {
		meta := StageMetas[state.CurrentStage]
		if meta == nil {
			continue
		}
		// 沉睡阈值：默认基于 DefaultFollowup*3；如无则用阶段级硬编码
		threshold := meta.DefaultFollowup * 3
		if threshold == 0 {
			threshold = stageDefaultSleepThreshold(state.CurrentStage)
		}
		if threshold == 0 {
			continue // lost / stranger 不处理
		}
		// 用 StageSince + LastTouchAt 两者中更早的时间（更早的表示更久没互动）
		ref := state.StageSince
		if !state.LastTouchAt.IsZero() && state.LastTouchAt.Before(ref) {
			ref = state.LastTouchAt
		}
		// 如果 LastTouchAt 是零值（从未互动过），直接用 StageSince
		if state.LastTouchAt.IsZero() {
			ref = state.StageSince
		}
		if now.Sub(ref) > threshold {
			event := &JourneyEvent{
				Type:       "auto_sleep",
				FromStage:  state.CurrentStage,
				ToStage:    StageSleeping,
				Reason:     fmt.Sprintf("超 %v 未互动（阶段=%s）", threshold, state.CurrentStage),
				Source:     "system",
				OperatorID: "system",
				Timestamp:  now,
			}
			state.CurrentStage = StageSleeping
			state.StageSince = now
			state.StageHistory = append(state.StageHistory, *event)
			s.applyStageSideEffects(ctx, state, StageSleeping)
			wokeUp = append(wokeUp, cid)
			snapshots[cid] = cloneJourneyState(state)
		}
	}
	s.mu.Unlock()
	// 锁外统一持久化（最佳努力，Redis 不可用不影响主流程）
	for cid, snap := range snapshots {
		cs := snap
		s.persistState(ctx, &cs)
		_ = cid
	}
	return wokeUp
}

// stageDefaultSleepThreshold 阶段级硬编码沉睡阈值
// 商业逻辑：成交/复购阶段虽然 DefaultFollowup=0（"下一次主动触达"无时间表），
// 但仍需定期唤醒沉睡客户。例如：成交后 90 天 → 沉睡；售后 60 天 → 沉睡
func stageDefaultSleepThreshold(stage JourneyStage) time.Duration {
	switch stage {
	case StageWon, StageAfterSale:
		return 90 * 24 * time.Hour
	case StageRepurchase:
		return 60 * 24 * time.Hour
	case StageQuoted:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// JourneyStageOverview 阶段总览
type JourneyStageOverview struct {
	Stage        JourneyStage `json:"stage"`
	Label        string       `json:"label"`
	Count        int          `json:"count"`
	Rate         float64      `json:"rate"`           // 占总客户比例 %
	AvgStayHours float64      `json:"avg_stay_hours"` // 平均停留小时
}

// JourneyOverview 客户旅程总览
type JourneyOverview struct {
	TotalCustomers int                    `json:"total_customers"`
	Stages         []JourneyStageOverview `json:"stages"`
	TotalEvents    int                    `json:"total_events"`
	GeneratedAt    time.Time              `json:"generated_at"`
}

// GetOverview 获取全旅程总览（各阶段客户数 + 转化率）
func (s *CustomerJourneyService) GetOverview(ctx context.Context) *JourneyOverview {
	s.mu.RLock()
	defer s.mu.RUnlock()

	overview := &JourneyOverview{
		Stages:      make([]JourneyStageOverview, 0, len(AllStages)),
		GeneratedAt: time.Now(),
	}
	overview.TotalCustomers = len(s.states)

	// 统计每个阶段的客户数与累计停留时间
	stageCounts := make(map[JourneyStage]int, len(AllStages))
	stageStaySum := make(map[JourneyStage]float64, len(AllStages))
	now := time.Now()
	for _, state := range s.states {
		stageCounts[state.CurrentStage]++
		stay := now.Sub(state.StageSince).Hours()
		stageStaySum[state.CurrentStage] += stay
		overview.TotalEvents += len(state.StageHistory)
	}

	// 按 AllStages 顺序输出
	for _, st := range AllStages {
		count := stageCounts[st]
		var rate float64
		if overview.TotalCustomers > 0 {
			rate = float64(count) / float64(overview.TotalCustomers) * 100
		}
		var avgStay float64
		if count > 0 {
			avgStay = stageStaySum[st] / float64(count)
		}
		meta, _ := StageMetas[st]
		label := string(st)
		if meta != nil {
			label = meta.Label
		}
		overview.Stages = append(overview.Stages, JourneyStageOverview{
			Stage:        st,
			Label:        label,
			Count:        count,
			Rate:         rate,
			AvgStayHours: avgStay,
		})
	}
	return overview
}
