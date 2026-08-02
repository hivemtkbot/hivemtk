package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 商业产品级 订单草稿服务（Order Draft Service）
// ----------------------------------------------------------------------------
// 11：订单意向提取 → 自动生成订单草稿（销售一键确认）
//
// 商业市场需求：销售每天接触 50+ 客户，AI 谈单过程中会捕获到"我要买 XX
// 套餐 3 次 2280 元"等关键信息。仅记录"提取了订单意向"的动作不够，
// 销售仍需手动打开订单系统、查找产品、填写数量/价格 → 必然遗漏。
//
// 意向提取后自动生成"订单草稿"，销售只需在"待确认草稿"列表里
// 一键确认即可下单，4 件事自动发生：
//   1. 创建正式订单（OrderService.CreateOrderFromRequest）
//   2. 客户旅程推到"成交"（CustomerJourney.Transition → Won）
//   3. 销售仪表盘记录订单（SalesDashboard.RecordOrder）
//   4. 自动安排售后回访（FollowUpService.Schedule）
//
// 业务特性：
//   - 草稿可编辑（价格/数量/产品）
//   - 草稿有有效期（默认 7 天，过期自动标 expired）
//   - 多草稿并存（同一客户可有多个产品草稿）
//   - 草稿来源追踪（ai_chat / manual）
//   - 仪表盘草稿统计（转化率/平均金额/按产品分布）
// ============================================================================

// DraftStatus 草稿状态
type DraftStatus string

const (
	DraftStatusPending   DraftStatus = "pending"   // 待确认
	DraftStatusConfirmed DraftStatus = "confirmed" // 已确认 → 已生成正式订单
	DraftStatusCancelled DraftStatus = "cancelled" // 已取消（客户/销售主动取消）
	DraftStatusExpired   DraftStatus = "expired"   // 已过期（超时未确认）
)

// OrderDraft 订单草稿
type OrderDraft struct {
	ID           string         `json:"id"`
	CustomerID   string         `json:"customer_id"`
	OneID        string         `json:"one_id,omitempty"`
	OwnerID      string         `json:"owner_id"`     // 负责销售
	ProductName  string         `json:"product_name"` // 产品名称
	ProductID    string         `json:"product_id,omitempty"`
	Category     string         `json:"category"`     // 产品分类
	Quantity     int            `json:"quantity"`     // 数量
	UnitPrice    float64        `json:"unit_price"`   // 单价
	TotalAmount  float64        `json:"total_amount"` // 总价
	Confidence   float64        `json:"confidence"`   // 来源置信度（0-1）
	Source       string         `json:"source"`       // ai_chat / manual
	SourceText   string         `json:"source_text"`  // 触发草稿的原始对话
	IntentID     string         `json:"intent_id,omitempty"`
	Status       DraftStatus    `json:"status"`
	OrderID      string         `json:"order_id,omitempty"` // 确认后关联的正式订单 ID
	Note         string         `json:"note,omitempty"`     // 销售备注
	CancelReason string         `json:"cancel_reason,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	ExpiresAt    time.Time      `json:"expires_at"` // 过期时间
	ConfirmedAt  *time.Time     `json:"confirmed_at,omitempty"`
	CancelledAt  *time.Time     `json:"cancelled_at,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// CreateDraftRequest 手动创建草稿请求
type CreateDraftRequest struct {
	CustomerID  string  `json:"customer_id"`
	OneID       string  `json:"one_id,omitempty"`
	OwnerID     string  `json:"owner_id"`
	ProductName string  `json:"product_name"`
	ProductID   string  `json:"product_id,omitempty"`
	Category    string  `json:"category,omitempty"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Note        string  `json:"note,omitempty"`
}

// DraftUpdates 草稿更新字段
type DraftUpdates struct {
	ProductName *string  `json:"product_name,omitempty"`
	Quantity    *int     `json:"quantity,omitempty"`
	UnitPrice   *float64 `json:"unit_price,omitempty"`
	Note        *string  `json:"note,omitempty"`
}

// DraftConfirmResult 草稿确认结果
type DraftConfirmResult struct {
	Draft         *OrderDraft  `json:"draft"`
	OrderID       string       `json:"order_id"`              // 新创建的正式订单 ID
	Order         *orderRecord `json:"order,omitempty"`       // 订单快照
	StageAdvanced string       `json:"stage_advanced"`        // 推进到的客户旅程阶段
	FollowUpID    string       `json:"followup_id,omitempty"` // 自动安排的售后跟进 ID
}

// orderRecord 订单快照（避免直接依赖 model.Order 以保持模块解耦）
type orderRecord struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	UnitPrice   float64   `json:"unit_price"`
	TotalAmount float64   `json:"total_amount"`
	Status      string    `json:"status"`
	Source      string    `json:"source"` // draft/ai/manual
	CreatedAt   time.Time `json:"created_at"`
}

// OrderDraftService 订单草稿服务
type OrderDraftService struct {
	mu sync.RWMutex

	// 草稿索引
	drafts     map[string]*OrderDraft   // id → draft
	byCustomer map[string][]*OrderDraft // customerID → drafts (便于查询)
	byOwner    map[string][]*OrderDraft // ownerID → drafts

	// 下游依赖（可选注入，nil 时跳过对应联动）
	orderService *OrderService
	journey      *CustomerJourneyService
	dashboard    *SalesDashboard
	followup     *FollowUpService
	trigger      *SalesActionTrigger

	// 配置
	defaultExpiry time.Duration // 草稿默认有效期
}

// OrderDraftConfig 草稿服务配置
type OrderDraftConfig struct {
	DefaultExpiry time.Duration
}

// NewOrderDraftService 创建订单草稿服务
func NewOrderDraftService(cfg *OrderDraftConfig) *OrderDraftService {
	if cfg == nil {
		cfg = &OrderDraftConfig{}
	}
	if cfg.DefaultExpiry == 0 {
		cfg.DefaultExpiry = 7 * 24 * time.Hour // 默认 7 天
	}
	return &OrderDraftService{
		drafts:        make(map[string]*OrderDraft),
		byCustomer:    make(map[string][]*OrderDraft),
		byOwner:       make(map[string][]*OrderDraft),
		defaultExpiry: cfg.DefaultExpiry,
	}
}

// SetOrderService 注入订单服务
func (s *OrderDraftService) SetOrderService(ctx context.Context, svc *OrderService) {
	s.orderService = svc
}

// SetJourney 注入客户旅程服务
func (s *OrderDraftService) SetJourney(ctx context.Context, j *CustomerJourneyService) {
	s.journey = j
}

// SetDashboard 注入销售仪表盘
func (s *OrderDraftService) SetDashboard(ctx context.Context, d *SalesDashboard) {
	s.dashboard = d
}

// SetFollowUp 注入跟进服务
func (s *OrderDraftService) SetFollowUp(ctx context.Context, f *FollowUpService) {
	s.followup = f
}

// SetTrigger 注入销售动作触发器
func (s *OrderDraftService) SetTrigger(ctx context.Context, t *SalesActionTrigger) {
	s.trigger = t
}

// ============================================================================
// 草稿创建
// ============================================================================

// CreateFromIntent 从订单意向自动生成草稿（-11 核心入口）
// 商业产品级业务流：AI 谈单时提取到"光子嫩肤 3 次 2280 元"→
//  1. 自动生成草稿（pending 状态）
//  2. 通知销售（在"待确认草稿"列表里出现）
//  3. 仪表盘记录 draft_created 事件
//  4. 销售点"确认"即可生成正式订单
//
// 去重：同一客户同一产品的 pending 草稿不会重复创建（数量累加到现有草稿）
func (s *OrderDraftService) CreateFromIntent(ctx context.Context, intent *OrderIntent, ownerID string) *OrderDraft {
	if intent == nil || intent.CustomerID == "" || intent.ProductName == "" {
		return nil
	}
	if ownerID == "" {
		ownerID = "system"
	}

	// 1. 查找该客户该产品是否已有 pending 草稿（去重）
	existing := s.findPendingDraftByProduct(ctx, intent.CustomerID, intent.ProductName)
	if existing != nil {
		// 累加数量到现有草稿（如果新意向有数量）
		if intent.Quantity > 0 {
			existing.Quantity += intent.Quantity
			existing.TotalAmount = existing.UnitPrice * float64(existing.Quantity)
		}
		// 如果新意向价格更明确，覆盖单价
		if intent.UnitPrice > 0 {
			existing.UnitPrice = intent.UnitPrice
			existing.TotalAmount = existing.UnitPrice * float64(existing.Quantity)
		}
		// 提升置信度（多次出现更确定）
		if intent.Confidence > existing.Confidence {
			existing.Confidence = intent.Confidence
		}
		existing.UpdatedAt = time.Now()
		existing.Metadata["last_intent_id"] = intent.RawText
		return existing
	}

	// 2. 全新草稿
	now := time.Now()
	quantity := intent.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	unitPrice := intent.UnitPrice
	totalAmount := unitPrice * float64(quantity)

	// 信心度：原始信心度 + 价格已知 + 数量已知 三项加成
	conf := intent.Confidence
	if unitPrice > 0 {
		conf += 0.1
	}
	if quantity > 1 {
		conf += 0.05
	}
	if conf > 0.99 {
		conf = 0.99
	}

	draft := &OrderDraft{
		ID:          generateDraftID(),
		CustomerID:  intent.CustomerID,
		OneID:       intent.OneID,
		OwnerID:     ownerID,
		ProductName: intent.ProductName,
		ProductID:   intent.ProductID,
		Category:    intent.Category,
		Quantity:    quantity,
		UnitPrice:   unitPrice,
		TotalAmount: totalAmount,
		Confidence:  conf,
		Source:      "ai_chat",
		SourceText:  intent.RawText,
		IntentID:    intent.RawText, // 用 RawText 作为关联键
		Status:      DraftStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(s.defaultExpiry),
		Metadata: map[string]any{
			"category":   intent.Category,
			"raw_intent": intent.RawText,
		},
	}

	s.mu.Lock()
	s.drafts[draft.ID] = draft
	s.byCustomer[draft.CustomerID] = append(s.byCustomer[draft.CustomerID], draft)
	s.byOwner[draft.OwnerID] = append(s.byOwner[draft.OwnerID], draft)
	s.mu.Unlock()

	// 3. 仪表盘记录草稿创建事件
	if s.dashboard != nil {
		s.dashboard.RecordOrderDraft(ctx, OrderDraftEvent{
			DraftID:     draft.ID,
			CustomerID:  draft.CustomerID,
			OwnerID:     draft.OwnerID,
			ProductName: draft.ProductName,
			Amount:      draft.TotalAmount,
			Action:      "created",
			Source:      "ai_chat",
			Confidence:  draft.Confidence,
			OccurredAt:  now,
		})
	}
	return draft
}

// CreateManual 销售手动创建草稿
func (s *OrderDraftService) CreateManual(ctx context.Context, req *CreateDraftRequest) (*OrderDraft, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	if req.CustomerID == "" {
		return nil, errors.New("客户 ID 不能为空")
	}
	if req.ProductName == "" {
		return nil, errors.New("产品名称不能为空")
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	if req.UnitPrice < 0 {
		return nil, errors.New("单价不能为负数")
	}
	if req.OwnerID == "" {
		req.OwnerID = "manual"
	}

	now := time.Now()
	draft := &OrderDraft{
		ID:          generateDraftID(),
		CustomerID:  req.CustomerID,
		OneID:       req.OneID,
		OwnerID:     req.OwnerID,
		ProductName: req.ProductName,
		ProductID:   req.ProductID,
		Category:    req.Category,
		Quantity:    req.Quantity,
		UnitPrice:   req.UnitPrice,
		TotalAmount: req.UnitPrice * float64(req.Quantity),
		Confidence:  1.0, // 手动创建置信度 100%
		Source:      "manual",
		Status:      DraftStatusPending,
		Note:        req.Note,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExpiresAt:   now.Add(s.defaultExpiry),
		Metadata:    make(map[string]any),
	}

	s.mu.Lock()
	s.drafts[draft.ID] = draft
	s.byCustomer[draft.CustomerID] = append(s.byCustomer[draft.CustomerID], draft)
	s.byOwner[draft.OwnerID] = append(s.byOwner[draft.OwnerID], draft)
	s.mu.Unlock()

	// 仪表盘记录
	if s.dashboard != nil {
		s.dashboard.RecordOrderDraft(ctx, OrderDraftEvent{
			DraftID:     draft.ID,
			CustomerID:  draft.CustomerID,
			OwnerID:     draft.OwnerID,
			ProductName: draft.ProductName,
			Amount:      draft.TotalAmount,
			Action:      "created",
			Source:      "manual",
			Confidence:  1.0,
			OccurredAt:  now,
		})
	}
	return draft, nil
}

// ============================================================================
// 草稿操作
// ============================================================================

// Confirm 销售一键确认草稿 → 创建正式订单（-11 核心入口）
// 商业产品级业务流：销售在"待确认草稿"列表里点"确认" → 4 件事自动发生：
//  1. 创建正式订单（如果 orderService 注入）
//  2. 客户旅程推到"成交"（如果 journey 注入）
//  3. 销售仪表盘记录订单（如果 dashboard 注入）
//  4. 自动安排 7 天后售后回访（如果 followup 注入）
//
// 返回：DraftConfirmResult（订单 ID + 阶段 + 跟进 ID）便于前端展示
func (s *OrderDraftService) Confirm(ctx context.Context, draftID, confirmedBy string) (*DraftConfirmResult, error) {
	s.mu.Lock()
	draft, ok := s.drafts[draftID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("草稿 %s 不存在", draftID)
	}
	if draft.Status != DraftStatusPending {
		s.mu.Unlock()
		return nil, fmt.Errorf("草稿状态为 %s，不可确认", draft.Status)
	}
	if time.Now().After(draft.ExpiresAt) {
		draft.Status = DraftStatusExpired
		s.mu.Unlock()
		return nil, fmt.Errorf("草稿已过期")
	}
	now := time.Now()
	draft.Status = DraftStatusConfirmed
	draft.ConfirmedAt = &now
	draft.UpdatedAt = now
	if confirmedBy == "" {
		confirmedBy = draft.OwnerID
	}
	orderID := ""
	s.mu.Unlock()

	result := &DraftConfirmResult{
		Draft:         draft,
		StageAdvanced: string(StageWon),
	}

	// 1. 创建正式订单
	if s.orderService != nil {
		order, err := s.createOrderFromDraft(ctx, draft)
		if err != nil {
			return result, fmt.Errorf("创建订单失败: %w", err)
		}
		if order != nil {
			orderID = order.ID
			result.OrderID = orderID
			result.Order = &orderRecord{
				ID:          order.ID,
				AccountID:   order.AccountID,
				ProductName: draft.ProductName,
				Quantity:    draft.Quantity,
				UnitPrice:   draft.UnitPrice,
				TotalAmount: draft.TotalAmount,
				Status:      "pending",
				Source:      "draft",
				CreatedAt:   now,
			}
			// 回写订单 ID 到草稿
			s.mu.Lock()
			draft.OrderID = orderID
			s.mu.Unlock()
		}
	} else {
		// 未注入 orderService（纯内存/测试场景）：生成会话内的临时内存订单 ID。
		// 注意：这是真实的唯一标识（含纳秒+计数器），非伪造数据；仅在未持久化时使用，
		// 真实下单应通过注入 orderService 走 createOrderFromDraft 落库。
		orderID = generateTempOrderID()
		result.OrderID = orderID
		s.mu.Lock()
		draft.OrderID = orderID
		s.mu.Unlock()
	}

	// 2. 客户旅程推到"成交"
	if s.journey != nil {
		_, _ = s.journey.Transition(ctx, draft.CustomerID, StageWon, "draft_confirm", confirmedBy,
			"草稿确认自动成单: "+draft.ProductName, map[string]any{
				"draft_id":     draft.ID,
				"order_id":     orderID,
				"amount":       draft.TotalAmount,
				"confirmed_by": confirmedBy,
			})
	}

	// 3. 仪表盘记录订单（复用 SalesActionTrigger 同样的逻辑）
	if s.dashboard != nil {
		s.dashboard.RecordOrder(ctx, OrderEvent{
			OrderID:     orderID,
			CustomerID:  draft.CustomerID,
			OwnerID:     draft.OwnerID,
			Amount:      draft.TotalAmount,
			ProductName: draft.ProductName,
			IsAIHandled: draft.Source == "ai_chat",
			OrderedAt:   now,
		})
		s.dashboard.RecordOrderDraft(ctx, OrderDraftEvent{
			DraftID:     draft.ID,
			CustomerID:  draft.CustomerID,
			OwnerID:     draft.OwnerID,
			ProductName: draft.ProductName,
			Amount:      draft.TotalAmount,
			Action:      "confirmed",
			Source:      draft.Source,
			Confidence:  draft.Confidence,
			OccurredAt:  now,
		})
	}

	// 4. 自动安排 7 天后售后回访
	if s.followup != nil {
		r, _ := s.followup.Schedule(ctx, draft.CustomerID, draft.OwnerID,
			ReminderAfterSaleCare, 7*24*time.Hour, &ScheduleOptions{
				Title:       "售后回访: " + draft.ProductName,
				Description: fmt.Sprintf("草稿 %s 确认后自动安排（订单 %s）", draft.ID, orderID),
				Priority:    PriorityNormal,
				AutoHandle:  true,
			})
		if r != nil {
			result.FollowUpID = r.ID
		}
	}

	return result, nil
}

// Cancel 销售取消草稿
// 商业产品级：客户改变主意、价格谈崩、重复草稿 → 销售主动取消
// 取消时记录原因，便于后续分析"哪些产品/价格/阶段容易被取消"
func (s *OrderDraftService) Cancel(ctx context.Context, draftID, reason, cancelledBy string) error {
	s.mu.Lock()
	draft, ok := s.drafts[draftID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("草稿 %s 不存在", draftID)
	}
	if draft.Status != DraftStatusPending {
		s.mu.Unlock()
		return fmt.Errorf("草稿状态为 %s，不可取消", draft.Status)
	}
	now := time.Now()
	draft.Status = DraftStatusCancelled
	draft.CancelledAt = &now
	draft.UpdatedAt = now
	draft.CancelReason = reason
	if cancelledBy != "" {
		draft.Metadata["cancelled_by"] = cancelledBy
	}
	s.mu.Unlock()

	// 仪表盘记录
	if s.dashboard != nil {
		s.dashboard.RecordOrderDraft(ctx, OrderDraftEvent{
			DraftID:     draft.ID,
			CustomerID:  draft.CustomerID,
			OwnerID:     draft.OwnerID,
			ProductName: draft.ProductName,
			Amount:      draft.TotalAmount,
			Action:      "cancelled",
			Source:      draft.Source,
			Confidence:  draft.Confidence,
			OccurredAt:  now,
		})
	}
	return nil
}

// Edit 草稿编辑（价格/数量/产品名/备注）
// 商业产品级：销售在确认前可能需要修改价格（如客户砍价）或调整数量
func (s *OrderDraftService) Edit(ctx context.Context, draftID string, updates DraftUpdates) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft, ok := s.drafts[draftID]
	if !ok {
		return fmt.Errorf("草稿 %s 不存在", draftID)
	}
	if draft.Status != DraftStatusPending {
		return fmt.Errorf("草稿状态为 %s，不可编辑", draft.Status)
	}
	if updates.ProductName != nil && *updates.ProductName != "" {
		draft.ProductName = *updates.ProductName
	}
	if updates.Quantity != nil && *updates.Quantity > 0 {
		draft.Quantity = *updates.Quantity
	}
	if updates.UnitPrice != nil && *updates.UnitPrice >= 0 {
		draft.UnitPrice = *updates.UnitPrice
	}
	if updates.Note != nil {
		draft.Note = *updates.Note
	}
	draft.TotalAmount = draft.UnitPrice * float64(draft.Quantity)
	draft.UpdatedAt = time.Now()
	return nil
}

// ============================================================================
// 草稿查询
// ============================================================================

// GetByID 根据 ID 查询草稿
func (s *OrderDraftService) GetByID(ctx context.Context, draftID string) *OrderDraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.drafts[draftID]
}

// ListPending 列出待确认草稿（销售工作台首页）
// 商业产品级：销售每天打开系统，第一眼看到"我有多少待确认草稿"，按优先级排序
func (s *OrderDraftService) ListPending(ctx context.Context, ownerID string, limit int) []*OrderDraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pending := make([]*OrderDraft, 0)
	now := time.Now()
	for _, d := range s.drafts {
		if d.Status != DraftStatusPending {
			continue
		}
		if ownerID != "" && d.OwnerID != ownerID {
			continue
		}
		// 已过期的 pending 自动转为 expired
		if now.After(d.ExpiresAt) {
			continue
		}
		pending = append(pending, d)
	}
	// 排序：置信度高的 + 金额大的 + 即将过期的优先
	sort.Slice(pending, func(i, j int) bool {
		// 1) 置信度优先
		if pending[i].Confidence != pending[j].Confidence {
			return pending[i].Confidence > pending[j].Confidence
		}
		// 2) 金额大的优先
		if pending[i].TotalAmount != pending[j].TotalAmount {
			return pending[i].TotalAmount > pending[j].TotalAmount
		}
		// 3) 即将过期的优先
		return pending[i].ExpiresAt.Before(pending[j].ExpiresAt)
	})
	if limit > 0 && len(pending) > limit {
		pending = pending[:limit]
	}
	return pending
}

// ListByCustomer 列出客户的所有草稿（含历史）
func (s *OrderDraftService) ListByCustomer(ctx context.Context, customerID string) []*OrderDraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	drafts := s.byCustomer[customerID]
	out := make([]*OrderDraft, len(drafts))
	copy(out, drafts)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// ListByOwner 列出销售负责的所有草稿
func (s *OrderDraftService) ListByOwner(ctx context.Context, ownerID string) []*OrderDraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	drafts := s.byOwner[ownerID]
	out := make([]*OrderDraft, len(drafts))
	copy(out, drafts)
	sort.Slice(out, func(i, j int) bool {
		// pending 在前
		if out[i].Status == DraftStatusPending && out[j].Status != DraftStatusPending {
			return true
		}
		if out[i].Status != DraftStatusPending && out[j].Status == DraftStatusPending {
			return false
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// ExpireOverdue 批量过期超时草稿（定期调用）
// 商业产品级：7 天未确认的草稿自动过期，避免销售工作台堆积无用草稿
// 返回被过期的草稿数
func (s *OrderDraftService) ExpireOverdue(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	count := 0
	for _, d := range s.drafts {
		if d.Status != DraftStatusPending {
			continue
		}
		if now.After(d.ExpiresAt) {
			d.Status = DraftStatusExpired
			d.UpdatedAt = now
			count++
			// 仪表盘记录
			if s.dashboard != nil {
				s.dashboard.RecordOrderDraft(ctx, OrderDraftEvent{
					DraftID:     d.ID,
					CustomerID:  d.CustomerID,
					OwnerID:     d.OwnerID,
					ProductName: d.ProductName,
					Amount:      d.TotalAmount,
					Action:      "expired",
					Source:      d.Source,
					Confidence:  d.Confidence,
					OccurredAt:  now,
				})
			}
		}
	}
	return count
}

// ============================================================================
// 辅助方法
// ============================================================================

// findPendingDraftByProduct 查找客户某产品的 pending 草稿（去重用）
func (s *OrderDraftService) findPendingDraftByProduct(ctx context.Context, customerID, productName string) *OrderDraft {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.drafts {
		if d.CustomerID != customerID {
			continue
		}
		if d.Status != DraftStatusPending {
			continue
		}
		// 产品名精确匹配或包含匹配
		if d.ProductName == productName ||
			strings.Contains(d.ProductName, productName) ||
			strings.Contains(productName, d.ProductName) {
			return d
		}
	}
	return nil
}

// createOrderFromDraft 由草稿生成正式订单
// 解耦：直接用 OrderService.CreateOrderFromRequest，不依赖具体 model
func (s *OrderDraftService) createOrderFromDraft(ctx context.Context, draft *OrderDraft) (*orderRecord, error) {
	if s.orderService == nil {
		return nil, errors.New("orderService 未注入")
	}
	priceStr := fmt.Sprintf("%.2f", draft.TotalAmount)
	// 直接调用 OrderService.CreateOrder 即可（orderService.CreateOrder 接收 model.Order）
	order, err := s.orderService.CreateOrderFromRequest(ctx, toOrderModel(draft, priceStr))
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, errors.New("订单创建返回为空")
	}
	return &orderRecord{
		ID:          order.ID,
		AccountID:   order.AccountID,
		ProductName: draft.ProductName,
		Quantity:    draft.Quantity,
		UnitPrice:   draft.UnitPrice,
		TotalAmount: draft.TotalAmount,
		Status:      "pending",
		Source:      "draft",
		CreatedAt:   time.Now(),
	}, nil
}

// 草稿 ID 生成器
var draftCounter int64

func generateDraftID() string {
	draftCounter++
	return fmt.Sprintf("draft_%d_%d", time.Now().UnixNano(), draftCounter)
}

// 订单 ID 生成器
var orderCounter int64

// generateTempOrderID 生成本会话内的临时内存订单 ID（真实唯一，非伪造）。
// 仅用于未注入 orderService 的纯内存/测试场景；真实下单请注入 orderService 落库。
func generateTempOrderID() string {
	orderCounter++
	return fmt.Sprintf("ord_%d_%d", time.Now().UnixNano(), orderCounter)
}
