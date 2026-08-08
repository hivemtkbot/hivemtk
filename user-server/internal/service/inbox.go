package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	dbUtil "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// 统一收件箱 - 业务错误
var (
	ErrInboxEmptyMerchant       = errors.New("user_id is required")
	ErrInboxInvalidPlatform     = errors.New("invalid platform")
	ErrInboxInvalidAccount      = errors.New("invalid account_id")
	ErrInboxInvalidCustomer     = errors.New("invalid customer_id")
	ErrInboxInvalidAssignTo     = errors.New("invalid assign_to")
	ErrInboxInvalidAction       = errors.New("invalid assignment action")
	ErrInboxConversationExist   = errors.New("conversation already exists for this account/customer")
	ErrInboxConversationMissing = errors.New("conversation not found")
	ErrInboxInvalidStatus       = errors.New("invalid conversation status")
	ErrInboxRepoNotReady        = errors.New("inbox repository not ready")
)

// 统一收件箱会话状态
const (
	InboxStatusUnread   = "unread"
	InboxStatusOpen     = "open"
	InboxStatusAssigned = "assigned"
	InboxStatusClosed   = "closed"
)

// 收件箱超时未响应阈值：客户最后一条消息超过该时长且无我方回复，即视为“超时未响应”。
const InboxOverdueThreshold = 30 * time.Minute

// 收件箱对账模式
const (
	ReconcileModeUnread   = "unread"   // 以 message_hub 最后一条消息为事实源，重算未读/状态（修正历史数据）
	ReconcileModeOverdue  = "overdue"  // 超时未响应：转在线人工坐席处理（否则默认 AI 处理）
	ReconcileModeBackfill = "backfill" // 回填：修正 NULL/空 conversation_id 脏数据 + 为历史会话补建 inbox_conversations（消除 sync_gap 缺口）
)

// 分配动作
const (
	InboxActionAssign   = "assign"
	InboxActionReassign = "reassign"
	InboxActionRelease  = "release"
	InboxActionClose    = "close"
	InboxActionReopen   = "reopen"
)

// 接收方类型
const (
	InboxAssignToHuman = "human"
	InboxAssignToSOP   = "sop"
	InboxAssignToAI    = "ai"
)

// 收件箱会话来源
const (
	InboxFromCustomer = "customer"
	InboxFromStaff    = "staff"
	InboxFromAI       = "ai"
)

// 默认负载阈值：单客服最多承接会话数
const InboxDefaultStaffLoadLimit = 30

// InboxService 统一收件箱服务
type InboxService struct {
	inboxRepo      *repository.InboxConversationRepository
	assignmentRepo *repository.InboxAssignmentRepository
	hubRepo        *repository.MessageHubRepository
	sessionMsgRepo *repository.SessionMessageRepository
	mu             sync.RWMutex
	staffLoadCache map[string]int // staffUserID -> 当前承接数（缓存）
}

// NewInboxService 创建统一收件箱服务(无参,内部用 dbUtil.GetDB())
func NewInboxService() *InboxService {
	return NewInboxServiceWithDB(dbUtil.GetDB())
}

// NewInboxServiceWithDB 创建带 DB 的统一收件箱服务(显式注入 db,兼容旧调用)
//
// 五层架构 §三.5：构造函数保留 db *gorm.DB 参数（调用方不变），
// 内部创建 repository 实例，service 不再持有 db。
// db 为 nil 时（如单元测试）repo 字段保持 nil，方法调用做无操作短路。
func NewInboxServiceWithDB(db *gorm.DB) *InboxService {
	var inboxRepo *repository.InboxConversationRepository
	var assignmentRepo *repository.InboxAssignmentRepository
	var hubRepo *repository.MessageHubRepository
	var sessionMsgRepo *repository.SessionMessageRepository
	if db != nil {
		inboxRepo = repository.NewInboxConversationRepositoryWithDB(db)
		assignmentRepo = repository.NewInboxAssignmentRepositoryWithDB(db)
		hubRepo = repository.NewMessageHubRepositoryWithDB(db)
		sessionMsgRepo = repository.NewSessionMessageRepositoryWithDB(db)
	}
	return &InboxService{
		inboxRepo:      inboxRepo,
		assignmentRepo: assignmentRepo,
		hubRepo:        hubRepo,
		sessionMsgRepo: sessionMsgRepo,
		staffLoadCache: make(map[string]int),
	}
}

// InboxQuery 会话列表查询条件
type InboxQuery struct {
	Platform    string
	AccountID   string
	CustomerID  string
	Keyword     string
	Status      string
	AssignedTo  string
	AssignedSOP uint
	Pinned      *bool
	Starred     *bool
	Muted       *bool
	Page        int
	PageSize    int
	OrderBy     string // pinned_desc, latest_desc, unread_desc
}

// InboxAssignRequest 分配请求
type InboxAssignRequest struct {
	ConversationID uint
	Action         string // assign/reassign/release/close/reopen
	ToType         string // human/sop/ai
	ToUserID       string
	ToSOPID        uint
	OperatorID     string
	Remark         string
}

// InboxStats 收件箱统计
type InboxStats struct {
	Total        int64            `json:"total"`
	Unread       int64            `json:"unread"`
	Open         int64            `json:"open"`
	Assigned     int64            `json:"assigned"`
	Closed       int64            `json:"closed"`
	ByPlatform   map[string]int64 `json:"by_platform"`
	ByAssignedTo map[string]int64 `json:"by_assigned_to"`
	OverdueCount int64            `json:"overdue_count"`
}

// ConversationKey 会话唯一键
type ConversationKey struct {
	Platform   string
	AccountID  string
	CustomerID string
}

// Key 计算唯一键字符串
func (k ConversationKey) Key(ctx context.Context) string {
	return k.Platform + "|" + k.AccountID + "|" + k.CustomerID
}

// UpsertFromMessage 根据消息自动 upsert 会话（保留旧接口）
func (s *InboxService) UpsertFromMessage(ctx context.Context, msg *model.MessageHub) error {
	if msg == nil {
		return nil
	}
	_, err := s.UpsertFromHubMessage(ctx, msg)
	return err
}

// UpsertFromHubMessage 通用 upsert
func (s *InboxService) UpsertFromHubMessage(ctx context.Context, msg *model.MessageHub) (*model.InboxConversation, error) {
	if s.inboxRepo == nil {
		return nil, nil
	}
	if msg == nil {
		return nil, nil
	}
	if err := s.upsertInternal(ctx, msg); err != nil {
		return nil, err
	}
	// 取出会话
	cid := msg.SenderID
	if msg.Direction == "outbound" && msg.ReceiverID != "" {
		cid = msg.ReceiverID
	}
	conv, err := s.inboxRepo.FindByPlatformAccountCustomer(ctx, msg.Platform, msg.AccountID, cid)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

// UpsertFromHubMessageTx 与 UpsertFromHubMessage 等价，但跨表事务版本：
// 通过 hubRepo.CreateWithInboxTx 把 message_hub.Create + inbox_conversations.UpsertFromMessage
// 包在同一 DB 事务内，避免"消息已落 message_hub 但 inbox_conversations 缺失"的极端不一致。
//
// 修复（2026-08-05 审计 P1）：原 persistMessage 两步非原子，inbox 写失败仅 Warn 日志。
//
// 用法（在 InboxIngressService.persistMessage 中）：
//
//	err := s.inboxSvc.UpsertFromHubMessageTx(ctx, hub, s.hubRepo)
//
// hubRepo 由调用方传入（InboxIngressService 持有 hubRepo，InboxService 不持有）。
// hub 已在事务中 Create 成功（msg.ID 非零）— 调用方据此决定后续是否取会话。
// 返回 conv 为会话快照（事务提交后再查一次），失败时 conv=nil + err。
func (s *InboxService) UpsertFromHubMessageTx(ctx context.Context, msg *model.MessageHub, hubRepo *repository.MessageHubRepository) (*model.InboxConversation, error) {
	if s.inboxRepo == nil || msg == nil {
		return nil, nil
	}
	if hubRepo == nil {
		// 兜底：无 hubRepo 时退化为非事务版本
		return s.UpsertFromHubMessage(ctx, msg)
	}
	// 复用 upsertInternal 的 input 构造逻辑
	if msg.Platform == "" || msg.AccountID == "" || msg.SenderID == "" {
		return nil, ErrInboxEmptyMerchant
	}
	customerID := msg.SenderID
	if msg.Direction == "outbound" && msg.ReceiverID != "" {
		customerID = msg.ReceiverID
	}
	if customerID == "" {
		return nil, ErrInboxInvalidCustomer
	}
	now := time.Now()
	preview := msg.Content
	if len(preview) > 200 {
		preview = preview[:200]
	}
	from := InboxFromCustomer
	if msg.Direction == "outbound" {
		if msg.IsAIReply {
			from = InboxFromAI
		} else {
			from = InboxFromStaff
		}
	}
	input := repository.UpsertFromMessageInput{
		Platform:           msg.Platform,
		AccountID:          msg.AccountID,
		CustomerID:         customerID,
		CustomerName:       msg.SenderName,
		ConversationID:     msg.ConversationID,
		LastMessageID:      msg.ID,
		LastMessagePreview: preview,
		LastMessageAt:      now,
		LastMessageFrom:    from,
	}
	// 跨表事务：hubRepo.CreateWithInboxTx 内部开 tx，先 Create(hub)，再调 inboxRepo.UpsertFromMessageTx(tx, input)
	if err := hubRepo.CreateWithInboxTx(ctx, msg, s.inboxRepo, input); err != nil {
		return nil, err
	}
	// 事务提交后取出会话快照（msg.ID 此时已填充）
	conv, err := s.inboxRepo.FindByPlatformAccountCustomer(ctx, msg.Platform, msg.AccountID, customerID)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

// upsertInternal 内部 upsert（事务封装在 repository.UpsertFromMessage 中）
func (s *InboxService) upsertInternal(ctx context.Context, msg *model.MessageHub) error {
	if s.inboxRepo == nil || msg == nil {
		return nil
	}
	if msg.Platform == "" || msg.AccountID == "" || msg.SenderID == "" {
		return ErrInboxEmptyMerchant
	}
	// 默认以 sender 作为 customerId。outbound 消息时使用 receiver
	customerID := msg.SenderID
	if msg.Direction == "outbound" {
		if msg.ReceiverID != "" {
			customerID = msg.ReceiverID
		}
	}
	if customerID == "" {
		return ErrInboxInvalidCustomer
	}

	now := time.Now()
	preview := msg.Content
	if len(preview) > 200 {
		preview = preview[:200]
	}
	from := InboxFromCustomer
	if msg.Direction == "outbound" {
		if msg.IsAIReply {
			from = InboxFromAI
		} else {
			from = InboxFromStaff
		}
	}

	return s.inboxRepo.UpsertFromMessage(ctx, repository.UpsertFromMessageInput{
		Platform:           msg.Platform,
		AccountID:          msg.AccountID,
		CustomerID:         customerID,
		CustomerName:       msg.SenderName,
		ConversationID:     msg.ConversationID,
		LastMessageID:      msg.ID,
		LastMessagePreview: preview,
		LastMessageAt:      now,
		LastMessageFrom:    from,
	})
}

// List 会话列表
func (s *InboxService) List(ctx context.Context, q InboxQuery) ([]*model.InboxConversation, int64, error) {
	if s.inboxRepo == nil {
		return []*model.InboxConversation{}, 0, nil
	}
	return s.inboxRepo.ListByQuery(ctx, repository.InboxConversationQuery{
		Platform:    q.Platform,
		AccountID:   q.AccountID,
		CustomerID:  q.CustomerID,
		Keyword:     q.Keyword,
		Status:      q.Status,
		AssignedTo:  q.AssignedTo,
		AssignedSOP: q.AssignedSOP,
		Pinned:      q.Pinned,
		Starred:     q.Starred,
		Muted:       q.Muted,
		Page:        q.Page,
		PageSize:    q.PageSize,
		OrderBy:     q.OrderBy,
	})
}

// GetByID 通过 ID 获取会话
func (s *InboxService) GetByID(ctx context.Context, id uint) (*model.InboxConversation, error) {
	if s.inboxRepo == nil {
		return nil, nil
	}
	conv, err := s.inboxRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInboxConversationMissing
		}
		return nil, err
	}
	return conv, nil
}

// MarkRead 标记会话已读（重置未读计数）
func (s *InboxService) MarkRead(ctx context.Context, conversationID uint) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.MarkRead(ctx, conversationID)
}

// Pin / Unpin
func (s *InboxService) Pin(ctx context.Context, conversationID uint, pinned bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "pinned", pinned)
}

// Star / Unstar
func (s *InboxService) Star(ctx context.Context, conversationID uint, starred bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "starred", starred)
}

// Mute / Unmute
func (s *InboxService) Mute(ctx context.Context, conversationID uint, muted bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "muted", muted)
}

// AddTag / RemoveTag
func (s *InboxService) AddTag(ctx context.Context, conversationID uint, tag string) error {
	if s.inboxRepo == nil || tag == "" {
		return nil
	}
	conv, err := s.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.Tags == nil {
		conv.Tags = model.JSONArray{}
	}
	for _, t := range conv.Tags {
		if str, ok := t.(string); ok && str == tag {
			return nil
		}
	}
	conv.Tags = append(conv.Tags, tag)
	return s.inboxRepo.UpdateField(ctx, conversationID, "tags", conv.Tags)
}

func (s *InboxService) RemoveTag(ctx context.Context, conversationID uint, tag string) error {
	if s.inboxRepo == nil || tag == "" {
		return nil
	}
	conv, err := s.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if len(conv.Tags) == 0 {
		return nil
	}
	var newTags model.JSONArray
	for _, t := range conv.Tags {
		if str, ok := t.(string); ok && str != tag {
			newTags = append(newTags, t)
		}
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "tags", newTags)
}

// Assign 分配会话（assign/reassign/release/close/reopen）
//
// 五层架构 §三.5：DB 事务（更新会话 + 写入历史）封装在 repository.AssignTx，
// 负载缓存属于内存数据，不属于 DB 操作，故保留在 service 层。
// 缓存更新在事务提交后执行，避免事务回滚后缓存与 DB 不一致。
func (s *InboxService) Assign(ctx context.Context, req InboxAssignRequest) (*model.InboxAssignment, error) {
	if s.inboxRepo == nil {
		return nil, nil
	}
	if req.ConversationID == 0 {
		return nil, ErrInboxConversationMissing
	}
	allowedActions := map[string]bool{
		InboxActionAssign: true, InboxActionReassign: true,
		InboxActionRelease: true, InboxActionClose: true, InboxActionReopen: true,
	}
	if !allowedActions[req.Action] {
		return nil, ErrInboxInvalidAction
	}
	if req.Action == InboxActionAssign || req.Action == InboxActionReassign {
		allowed := map[string]bool{InboxAssignToHuman: true, InboxAssignToSOP: true, InboxAssignToAI: true}
		if !allowed[req.ToType] {
			return nil, ErrInboxInvalidAssignTo
		}
		if req.ToType == InboxAssignToHuman && req.ToUserID == "" {
			return nil, ErrInboxInvalidAssignTo
		}
		if req.ToType == InboxAssignToSOP && req.ToSOPID == 0 {
			return nil, ErrInboxInvalidAssignTo
		}
	}

	out, err := s.inboxRepo.AssignTx(ctx, repository.AssignTxInput{
		ConversationID: req.ConversationID,
		Action:         req.Action,
		ToType:         req.ToType,
		ToUserID:       req.ToUserID,
		ToSOPID:        req.ToSOPID,
		OperatorID:     req.OperatorID,
		Remark:         req.Remark,
	})
	if err != nil {
		if err.Error() == "conversation not found" {
			return nil, ErrInboxConversationMissing
		}
		return nil, err
	}
	if out == nil {
		return nil, nil
	}

	// 缓存同步：释放旧负载 + 增加新负载（事务已提交，DB 与缓存一致）
	if out.OldAssignedTo != "" {
		s.releaseLoad(ctx, out.OldAssignedTo)
	}
	if out.NewAssignedTo != "" {
		s.addLoad(ctx, out.NewAssignedTo)
	}
	return out.History, nil
}

// AutoAssign 自动分配（负载最小优先）
func (s *InboxService) AutoAssign(ctx context.Context, conversationID uint, candidates []string, operatorID string) (*model.InboxAssignment, error) {
	if len(candidates) == 0 {
		return nil, ErrInboxInvalidAssignTo
	}
	staff, err := s.pickStaff(ctx, candidates)
	if err != nil {
		return nil, err
	}
	return s.Assign(ctx, InboxAssignRequest{
		ConversationID: conversationID,
		Action:         InboxActionAssign,
		ToType:         InboxAssignToHuman,
		ToUserID:       staff,
		OperatorID:     operatorID,
		Remark:         "auto-assign",
	})
}

// RoundRobinAssign 轮询分配
func (s *InboxService) RoundRobinAssign(ctx context.Context, conversationID uint, candidates []string, operatorID string) (*model.InboxAssignment, error) {
	if len(candidates) == 0 {
		return nil, ErrInboxInvalidAssignTo
	}
	staff, err := s.pickRoundRobin(ctx, candidates)
	if err != nil {
		return nil, err
	}
	return s.Assign(ctx, InboxAssignRequest{
		ConversationID: conversationID,
		Action:         InboxActionAssign,
		ToType:         InboxAssignToHuman,
		ToUserID:       staff,
		OperatorID:     operatorID,
		Remark:         "round-robin",
	})
}

// StaffLoad 客服当前负载
func (s *InboxService) StaffLoad(ctx context.Context, staffUserID string) (int, error) {
	if s.inboxRepo == nil {
		return 0, nil
	}
	n, err := s.inboxRepo.CountByAssignedToStatus(ctx, staffUserID, []string{InboxStatusAssigned, InboxStatusOpen})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// BatchAssign 批量分配
func (s *InboxService) BatchAssign(ctx context.Context, reqs []InboxAssignRequest) ([]*model.InboxAssignment, []error) {
	results := make([]*model.InboxAssignment, 0, len(reqs))
	errs := make([]error, 0, len(reqs))
	for i := range reqs {
		r, e := s.Assign(ctx, reqs[i])
		results = append(results, r)
		errs = append(errs, e)
	}
	return results, errs
}

// ListAssignments 历史分配
func (s *InboxService) ListAssignments(ctx context.Context, conversationID uint, page, pageSize int) ([]*model.InboxAssignment, int64, error) {
	if s.assignmentRepo == nil {
		return []*model.InboxAssignment{}, 0, nil
	}
	return s.assignmentRepo.ListByConversationID(ctx, conversationID, page, pageSize)
}

// GetStats 收件箱统计
func (s *InboxService) GetStats(ctx context.Context) (*InboxStats, error) {
	if s.inboxRepo == nil {
		return &InboxStats{ByPlatform: map[string]int64{}, ByAssignedTo: map[string]int64{}}, nil
	}
	validStatuses := []string{InboxStatusUnread, InboxStatusOpen, InboxStatusAssigned, InboxStatusClosed}
	activeStatuses := []string{InboxStatusAssigned, InboxStatusOpen}
	threshold := time.Now().Add(-30 * time.Minute)

	res, err := s.inboxRepo.GetStats(ctx, validStatuses, activeStatuses, InboxFromCustomer, threshold)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &InboxStats{ByPlatform: map[string]int64{}, ByAssignedTo: map[string]int64{}}, nil
	}
	return &InboxStats{
		Total:        res.Total,
		Unread:       res.Unread,
		Open:         res.Open,
		Assigned:     res.Assigned,
		Closed:       res.Closed,
		ByPlatform:   res.ByPlatform,
		ByAssignedTo: res.ByAssignedTo,
		OverdueCount: res.OverdueCount,
	}, nil
}

// ReconcileResult 对账结果
type ReconcileResult struct {
	Mode             string `json:"mode"`
	UnreadReconciled int64  `json:"unread_reconciled"`
	OverdueFound     int    `json:"overdue_found"`
	OverdueAssigned  int    `json:"overdue_assigned"`
	AssignedTo       string `json:"assigned_to,omitempty"`
	FixedNullConv    int64  `json:"fixed_null_conv_id"` // backfill：修正的 NULL/空 conversation_id 行数
	Backfilled       int64  `json:"backfilled"`         // backfill：补建的 inbox_conversations 会话数
	Message          string `json:"message"`
}

// Reconcile 收件箱对账 / 治理。
//
//   - mode=unread：以 message_hub 的“最后一条消息”为事实源，重算全部会话的未读计数与状态。
//     直接修正历史数据——AI 已回复的会话不再显示“未读”，其未读清零、状态由 unread 转 open（待处理/消息池）。
//   - mode=overdue：对“超时未响应”的会话（客户最后一条且超过 InboxOverdueThreshold，无我方回复），
//     转给在线人工坐席处理（实现“分配给人工客服”）；若无在线坐席则保留，默认由 AI/后续人工处理。
func (s *InboxService) Reconcile(ctx context.Context, mode string) (*ReconcileResult, error) {
	if s.inboxRepo == nil {
		return nil, ErrInboxRepoNotReady
	}
	res := &ReconcileResult{Mode: mode}
	switch mode {
	case ReconcileModeOverdue:
		threshold := time.Now().Add(-InboxOverdueThreshold)
		due, err := s.inboxRepo.FindOverdueConversations(ctx, threshold, 0)
		if err != nil {
			return nil, err
		}
		res.OverdueFound = len(due)
		agents, err := s.inboxRepo.ListOnlineAgentIDs(ctx)
		if err != nil {
			return nil, err
		}
		if len(agents) == 0 {
			res.Message = "无在线坐席，超时会话保留等待 AI/人工处理"
			return res, nil
		}
		// 轮询分配给在线坐席（默认集中转给同一在线坐席，便于其统一跟进）
		target := agents[0]
		res.AssignedTo = target
		for _, conv := range due {
			if conv.AssignedTo == target {
				continue
			}
			if _, err := s.Assign(ctx, InboxAssignRequest{
				ConversationID: conv.ID,
				Action:         InboxActionAssign,
				ToType:         InboxAssignToHuman,
				ToUserID:       target,
				OperatorID:     "system-reconcile",
				Remark:         "超时未响应自动转人工",
			}); err != nil {
				continue
			}
			res.OverdueAssigned++
		}
		res.Message = fmt.Sprintf("已将 %d/%d 条超时会话分配给在线坐席 %s 处理", res.OverdueAssigned, res.OverdueFound, target)
		return res, nil
	case ReconcileModeBackfill:
		return s.reconcileBackfill(ctx)
	default: // ReconcileModeUnread
		n, err := s.inboxRepo.ReconcileUnread(ctx)
		if err != nil {
			return nil, err
		}
		res.UnreadReconciled = n
		res.Message = fmt.Sprintf("已按消息事实源重算 %d 条会话的未读/状态", n)
		return res, nil
	}
}

// deriveBackfillConversationID 为缺 conversation_id 的历史消息派生兜底会话 ID。
//
// 与 inbox_ingress.go HandleIngressMessage 的兜底规则保持一致：
// ConversationID → Extra.account_id → platform:unknown。保证回填后的会话可被 UI 按
// conversation_id 正常聚合，且不会与现有兜底逻辑产生分歧。
func deriveBackfillConversationID(m *model.MessageHub) string {
	if m.ConversationID != "" {
		return m.ConversationID
	}
	accountID := ""
	if m.Extra != nil {
		if v, ok := m.Extra["account_id"].(string); ok {
			accountID = v
		}
	}
	if accountID == "" {
		accountID = "unknown"
	}
	return m.Platform + ":" + accountID
}

// reconcileBackfill 回填收件箱数据，消除 sync_gap 监控缺口：
//
//  1. 修正 message_hub 中 conversation_id 为 NULL/空的历史脏数据（统一收件箱特性上线前的
//     消息，ingest 兜底 conversation_id 的逻辑晚于这些数据的产生），派生 platform:account_id。
//  2. 为 message_hub 存在但 inbox_conversations 缺失的会话（按 conversation_id 左连接，
//     与 monitor sync_gap 检测语义一致）补建收件箱会话，复用 UpsertFromHubMessage。
//
// 根因：inbox_conversations 仅在“新消息入库”时物化，历史会话不会回填，导致监控持续误报缺口。
func (s *InboxService) reconcileBackfill(ctx context.Context) (*ReconcileResult, error) {
	res := &ReconcileResult{Mode: ReconcileModeBackfill}
	if s.hubRepo == nil {
		return nil, ErrInboxRepoNotReady
	}

	// 步骤 1：修正 NULL/空 conversation_id 脏数据
	nullRows, err := s.hubRepo.FindNullConversationIDRows(ctx)
	if err != nil {
		return nil, err
	}
	for i := range nullRows {
		m := nullRows[i]
		convID := deriveBackfillConversationID(&m)
		if err := s.hubRepo.UpdateConversationID(ctx, m.ID, convID); err != nil {
			logger.Errorf("reconcile backfill: 修正 NULL conversation_id 失败 id=%d: %v", m.ID, err)
			continue
		}
		res.FixedNullConv++
	}

	// 步骤 2：回填缺失的 inbox_conversations 历史会话
	missing, err := s.hubRepo.FindConversationIDsMissingInbox(ctx)
	if err != nil {
		return nil, err
	}
	for _, convID := range missing {
		latest, err := s.hubRepo.FindLatestByConversation(ctx, convID)
		if err != nil || latest == nil {
			continue
		}
		if _, err := s.UpsertFromHubMessage(ctx, latest); err != nil {
			logger.Warnf("reconcile backfill: 会话 %s 回填收件箱失败（已跳过）: %v", convID, err)
			continue
		}
		res.Backfilled++
	}

	res.Message = fmt.Sprintf("已修正 %d 条 NULL/空 conversation_id，补建 %d 个收件箱会话",
		res.FixedNullConv, res.Backfilled)
	logger.Infof("reconcile backfill 完成: %s", res.Message)
	return res, nil
}

// GetMessagesByConversation 拉取会话下的消息。
//
// 统一收件箱的消息来源有两套存储，必须合并后才能给坐席看到完整会话流：
//  1. message_hub：webhook / 渠道接入（企微、抖音等）的消息中台；
//  2. session_messages：网页 widget 访客消息 + 坐席在客服会话里的回复，
//     以 session_id 关联（InboxConversation.ConversationID == SessionMessage.SessionID）。
//
// 历史实现只读 message_hub，导致网页端发的消息在统一收件箱点开后空白。
func (s *InboxService) GetMessagesByConversation(ctx context.Context, conversationID uint, page, pageSize int) ([]map[string]any, int64, error) {
	if s.inboxRepo == nil {
		return []map[string]any{}, 0, nil
	}
	conv, err := s.GetByID(ctx, conversationID)
	if err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	// 1) 消息中台（渠道接入）
	var hubs []*model.MessageHub
	if s.hubRepo != nil {
		hubs, _ = s.hubRepo.ListByConversationContext(ctx, conv.Platform, conv.AccountID, conv.CustomerID)
	}

	// 2) 客服会话实时消息流（网页 widget / 坐席回复）
	var sms []*model.SessionMessage
	if s.sessionMsgRepo != nil && s.sessionMsgRepo.HasTable(ctx) {
		sms, _ = s.sessionMsgRepo.ListAllBySessionID(ctx, conv.ConversationID)
	}

	type mergedMsg struct {
		ts   time.Time
		data map[string]any
	}
	merged := make([]mergedMsg, 0, len(hubs)+len(sms))
	for _, h := range hubs {
		merged = append(merged, mergedMsg{
			ts: h.SentAt,
			data: map[string]any{
				"id":              h.ID,
				"source":          "hub",
				"msg_id":          h.MsgID,
				"conversation_id": h.ConversationID,
				"platform":        h.Platform,
				"account_id":      h.AccountID,
				"sender_id":       h.SenderID,
				"sender_name":     h.SenderName,
				"receiver_id":     h.ReceiverID,
				"content":         h.Content,
				"content_type":    h.MsgType,
				"media_url":       h.MediaURL,
				"is_ai_reply":     h.IsAIReply,
				"is_read":         h.IsRead,
				"sent_at":         h.SentAt,
				"created_at":      h.CreatedAt,
			},
		})
	}
	for _, sm := range sms {
		merged = append(merged, mergedMsg{
			ts: sm.CreatedAt,
			data: map[string]any{
				"id":              sm.ID,
				"source":          "session",
				"conversation_id": sm.SessionID,
				"sender_id":       sm.SenderID,
				"sender_name":     sm.SenderName,
				"sender_type":     sm.SenderType,
				"content":         sm.Content,
				"content_type":    sm.ContentType,
				"media_url":       sm.MediaURL,
				"is_ai_reply":     sm.SenderType == "ai",
				"is_read":         sm.IsRead,
				"sent_at":         sm.CreatedAt,
				"created_at":      sm.CreatedAt,
			},
		})
	}

	// 按时间倒序（最新消息在前）
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].ts.After(merged[j].ts)
	})

	total := int64(len(merged))
	start := (page - 1) * pageSize
	if start > len(merged) {
		start = len(merged)
	}
	end := start + pageSize
	if end > len(merged) {
		end = len(merged)
	}
	out := make([]map[string]any, 0, end-start)
	for _, m := range merged[start:end] {
		out = append(out, m.data)
	}
	return out, total, nil
}

// DeleteMessage 删除统一收件箱会话中的单条消息。
// source 取值："hub" 表示 message_hub 渠道接入记录，"session" 表示 session_messages 实时客服消息流。
// messageID 为对应数据表的自增 id（前端通过消息的 source 字段确定来源）。
func (s *InboxService) DeleteMessage(ctx context.Context, conversationID, messageID uint, source string) error {
	if s.inboxRepo == nil {
		return ErrInboxRepoNotReady
	}
	if _, err := s.GetByID(ctx, conversationID); err != nil {
		return err
	}
	switch source {
	case "hub":
		if s.hubRepo == nil {
			return ErrInboxRepoNotReady
		}
		if err := s.hubRepo.Delete(ctx, messageID); err != nil {
			return err
		}
	case "session":
		if s.sessionMsgRepo == nil || !s.sessionMsgRepo.HasTable(ctx) {
			return ErrInboxRepoNotReady
		}
		if err := s.sessionMsgRepo.Delete(ctx, messageID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("无效消息来源: %s", source)
	}
	return nil
}

// ---- 内部辅助 ----

// pickStaff 选择负载最小的客服
func (s *InboxService) pickStaff(ctx context.Context, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", ErrInboxInvalidAssignTo
	}
	loads := make([]int, len(candidates))
	s.mu.RLock()
	for i, c := range candidates {
		loads[i] = s.staffLoadCache[c]
	}
	s.mu.RUnlock()
	// 优先采用缓存值；若都 0，则查 DB
	allZero := true
	for _, v := range loads {
		if v > 0 {
			allZero = false
			break
		}
	}
	if allZero {
		for i, c := range candidates {
			n, _ := s.StaffLoad(ctx, c)
			loads[i] = n
		}
	}
	minIdx := 0
	for i := range loads {
		if loads[i] < loads[minIdx] {
			minIdx = i
		}
	}
	// 检查阈值
	if loads[minIdx] >= InboxDefaultStaffLoadLimit {
		return "", fmt.Errorf("all staff at capacity")
	}
	return candidates[minIdx], nil
}

// pickRoundRobin 轮询：按 last assignment 计数取最小
func (s *InboxService) pickRoundRobin(ctx context.Context, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", ErrInboxInvalidAssignTo
	}
	got := make(map[string]int64, len(candidates))
	if s.assignmentRepo != nil {
		counts, _ := s.assignmentRepo.GroupCountByToUserID(ctx, candidates, InboxActionAssign)
		for _, c := range counts {
			got[c.AssignedTo] = c.N
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return got[candidates[i]] < got[candidates[j]]
	})
	if got[candidates[0]] == 0 {
		// 避免所有人都没分配过：按原顺序
		return candidates[0], nil
	}
	return candidates[0], nil
}

func (s *InboxService) addLoad(ctx context.Context, staff string) {
	if staff == "" {
		return
	}
	s.mu.Lock()
	s.staffLoadCache[staff]++
	s.mu.Unlock()
}

func (s *InboxService) releaseLoad(ctx context.Context, staff string) {
	if staff == "" {
		return
	}
	s.mu.Lock()
	if s.staffLoadCache[staff] > 0 {
		s.staffLoadCache[staff]--
	}
	s.mu.Unlock()
}

func inferFromType(assignedTo string, assignedSOP uint) string {
	if assignedSOP > 0 {
		return InboxAssignToSOP
	}
	if assignedTo != "" {
		return InboxAssignToHuman
	}
	return "system"
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
