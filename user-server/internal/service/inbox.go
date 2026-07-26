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
)

// 统一收件箱会话状态
const (
	InboxStatusUnread   = "unread"
	InboxStatusOpen     = "open"
	InboxStatusAssigned = "assigned"
	InboxStatusClosed   = "closed"
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

	// 按时间正序（聊天流从旧到新）
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].ts.Before(merged[j].ts)
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
