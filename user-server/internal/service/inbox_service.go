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

	"gorm.io/gorm"
)

// 统一收件箱 - 业务错误
var (
	ErrInboxEmptyMerchant		= errors.New("user_id is required")
	ErrInboxInvalidPlatform		= errors.New("invalid platform")
	ErrInboxInvalidAccount		= errors.New("invalid account_id")
	ErrInboxInvalidCustomer		= errors.New("invalid customer_id")
	ErrInboxInvalidAssignTo		= errors.New("invalid assign_to")
	ErrInboxInvalidAction		= errors.New("invalid assignment action")
	ErrInboxConversationExist	= errors.New("conversation already exists for this account/customer")
	ErrInboxConversationMissing	= errors.New("conversation not found")
	ErrInboxInvalidStatus		= errors.New("invalid conversation status")
)

// 统一收件箱会话状态
const (
	InboxStatusUnread	= "unread"
	InboxStatusOpen		= "open"
	InboxStatusAssigned	= "assigned"
	InboxStatusClosed	= "closed"
)

// 分配动作
const (
	InboxActionAssign	= "assign"
	InboxActionReassign	= "reassign"
	InboxActionRelease	= "release"
	InboxActionClose	= "close"
	InboxActionReopen	= "reopen"
)

// 接收方类型
const (
	InboxAssignToHuman	= "human"
	InboxAssignToSOP	= "sop"
	InboxAssignToAI		= "ai"
)

// 收件箱会话来源
const (
	InboxFromCustomer	= "customer"
	InboxFromStaff		= "staff"
	InboxFromAI		= "ai"
)

// 默认负载阈值：单客服最多承接会话数
const InboxDefaultStaffLoadLimit = 30

// InboxService 统一收件箱服务
type InboxService struct {
	db		*gorm.DB
	mu		sync.RWMutex
	staffLoadCache	map[string]int	// staffUserID -> 当前承接数（缓存）
}

// NewInboxService 创建统一收件箱服务(无参,内部用 dbUtil.GetDB())
func NewInboxService() *InboxService {
	return NewInboxServiceWithDB(dbUtil.GetDB())
}

// NewInboxServiceWithDB 创建带 DB 的统一收件箱服务(显式注入 db,兼容旧调用)
func NewInboxServiceWithDB(db *gorm.DB) *InboxService {
	return &InboxService{
		db:		db,
		staffLoadCache:	make(map[string]int),
	}
}

// InboxQuery 会话列表查询条件
type InboxQuery struct {
	Platform	string
	AccountID	string
	CustomerID	string
	Keyword		string
	Status		string
	AssignedTo	string
	AssignedSOP	uint
	Pinned		*bool
	Starred		*bool
	Muted		*bool
	Page		int
	PageSize	int
	OrderBy		string	// pinned_desc, latest_desc, unread_desc
}

// InboxAssignRequest 分配请求
type InboxAssignRequest struct {
	ConversationID	uint
	Action		string	// assign/reassign/release/close/reopen
	ToType		string	// human/sop/ai
	ToUserID	string
	ToSOPID		uint
	OperatorID	string
	Remark		string
}

// InboxStats 收件箱统计
type InboxStats struct {
	Total		int64			`json:"total"`
	Unread		int64			`json:"unread"`
	Open		int64			`json:"open"`
	Assigned	int64			`json:"assigned"`
	Closed		int64			`json:"closed"`
	ByPlatform	map[string]int64	`json:"by_platform"`
	ByAssignedTo	map[string]int64	`json:"by_assigned_to"`
	OverdueCount	int64			`json:"overdue_count"`
}

// ConversationKey 会话唯一键
type ConversationKey struct {
	Platform	string
	AccountID	string
	CustomerID	string
}

// Key 计算唯一键字符串
func (k ConversationKey) Key(ctx context.Context)  string {
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
	if s.db == nil {
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
	var conv model.InboxConversation
	if err := s.db.Where(ctx, "platform = ? AND account_id = ? AND customer_id = ?", msg.Platform, msg.AccountID, cid).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

// upsertInternal 内部 upsert
func (s *InboxService) upsertInternal(ctx context.Context, msg *model.MessageHub) error {
	if s.db == nil || msg == nil {
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

	// 事务里查找并更新
	return s.db.Transaction(func(tx *gorm.DB) error {
		var conv model.InboxConversation
		err := tx.Where("platform = ? AND account_id = ? AND customer_id = ?",
			msg.Platform, msg.AccountID, customerID).
			First(&conv).Error

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

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 新建
			conv = model.InboxConversation{

				Platform:		msg.Platform,
				AccountID:		msg.AccountID,
				CustomerID:		customerID,
				CustomerName:		msg.SenderName,
				ConversationID:		msg.ConversationID,
				Status:			InboxStatusUnread,
				UnreadCount:		0,
				TotalCount:		1,
				LastMessageID:		msg.ID,
				LastMessagePreview:	preview,
				LastMessageAt:		&now,
				LastMessageFrom:	from,
			}
			// 客户首条消息视为未读
			if from == InboxFromCustomer {
				conv.UnreadCount = 1
			}
			return tx.Create(&conv).Error
		}
		if err != nil {
			return err
		}

		updates := map[string]any{
			"last_message_id":	msg.ID,
			"last_message_preview":	preview,
			"last_message_at":	now,
			"last_message_from":	from,
			"total_count":		conv.TotalCount + 1,
			"customer_name":	firstNonEmpty(msg.SenderName, conv.CustomerName),
			"conversation_id":	firstNonEmpty(msg.ConversationID, conv.ConversationID),
		}
		// 客户消息累加未读
		if from == InboxFromCustomer {
			updates["unread_count"] = conv.UnreadCount + 1
			// 如果会话处于 closed 状态，重新打开为 unread
			if conv.Status == InboxStatusClosed {
				updates["status"] = InboxStatusUnread
				updates["closed_at"] = nil
			} else if conv.Status == InboxStatusAssigned && conv.AssignedTo == "" {
				updates["status"] = InboxStatusUnread
			}
		}
		return tx.Model(&model.InboxConversation{}).
			Where("id = ?", conv.ID).
			Updates(updates).Error
	})
}

// List 会话列表
func (s *InboxService) List(ctx context.Context, q InboxQuery) ([]*model.InboxConversation, int64, error) {
	if s.db == nil {
		return []*model.InboxConversation{}, 0, nil
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&model.InboxConversation{})

	if q.Platform != "" {
		tx = tx.Where("platform = ?", q.Platform)
	}
	if q.AccountID != "" {
		tx = tx.Where("account_id = ?", q.AccountID)
	}
	if q.CustomerID != "" {
		tx = tx.Where("customer_id = ?", q.CustomerID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.AssignedTo != "" {
		tx = tx.Where("assigned_to = ?", q.AssignedTo)
	}
	if q.AssignedSOP > 0 {
		tx = tx.Where("assigned_to_sop = ?", q.AssignedSOP)
	}
	if q.Pinned != nil {
		tx = tx.Where("pinned = ?", *q.Pinned)
	}
	if q.Starred != nil {
		tx = tx.Where("starred = ?", *q.Starred)
	}
	if q.Muted != nil {
		tx = tx.Where("muted = ?", *q.Muted)
	}
	if q.Keyword != "" {
		// 模糊搜索最近消息预览
		tx = tx.Where("last_message_preview LIKE ?", "%"+q.Keyword+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 排序：pinned 置顶优先 + 未读数 + 最近消息时间
	orderBy := "pinned DESC, unread_count DESC, last_message_at DESC"
	switch q.OrderBy {
	case "latest_desc":
		orderBy = "last_message_at DESC"
	case "oldest_asc":
		orderBy = "last_message_at ASC"
	case "unread_desc":
		orderBy = "unread_count DESC, last_message_at DESC"
	case "pinned_first":
		orderBy = "pinned DESC, last_message_at DESC"
	}
	tx = tx.Order(orderBy)

	offset := (q.Page - 1) * q.PageSize
	var list []*model.InboxConversation
	if err := tx.Offset(offset).Limit(q.PageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetByID 通过 ID 获取会话
func (s *InboxService) GetByID(ctx context.Context, id uint) (*model.InboxConversation, error) {
	if s.db == nil {
		return nil, nil
	}
	var conv model.InboxConversation
	if err := s.db.First(ctx, &conv, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInboxConversationMissing
		}
		return nil, err
	}
	return &conv, nil
}

// MarkRead 标记会话已读（重置未读计数）
func (s *InboxService) MarkRead(ctx context.Context, conversationID uint) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"unread_count":	0,
			"status":	InboxStatusOpen,
		}).Error
}

// Pin / Unpin
func (s *InboxService) Pin(ctx context.Context, conversationID uint, pinned bool) error {
	if s.db == nil {
		return nil
	}
	return s.db.Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update("pinned", pinned).Error
}

// Star / Unstar
func (s *InboxService) Star(ctx context.Context, conversationID uint, starred bool) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update("starred", starred).Error
}

// Mute / Unmute
func (s *InboxService) Mute(ctx context.Context, conversationID uint, muted bool) error {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update("muted", muted).Error
}

// AddTag / RemoveTag
func (s *InboxService) AddTag(ctx context.Context, conversationID uint, tag string) error {
	if s.db == nil || tag == "" {
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
	return s.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update("tags", conv.Tags).Error
}

func (s *InboxService) RemoveTag(ctx context.Context, conversationID uint, tag string) error {
	if s.db == nil || tag == "" {
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
	return s.db.WithContext(ctx).Model(&model.InboxConversation{}).
		Where("id = ?", conversationID).
		Update("tags", newTags).Error
}

// Assign 分配会话（assign/reassign/release/close/reopen）
func (s *InboxService) Assign(ctx context.Context, req InboxAssignRequest) (*model.InboxAssignment, error) {
	if s.db == nil {
		return nil, nil
	}
	if req.ConversationID == 0 {
		return nil, ErrInboxConversationMissing
	}
	allowedActions := map[string]bool{
		InboxActionAssign:	true, InboxActionReassign: true,
		InboxActionRelease:	true, InboxActionClose: true, InboxActionReopen: true,
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

	var history *model.InboxAssignment
	err := s.db.Transaction( func(tx *gorm.DB) error {
		var conv model.InboxConversation
		if err := tx.First(&conv, req.ConversationID).Error; err != nil {
			return ErrInboxConversationMissing
		}

		// 释放旧负载缓存
		if conv.AssignedTo != "" {
			s.releaseLoad(ctx, conv.AssignedTo)
		}

		updates := map[string]any{}
		now := time.Now()
		switch req.Action {
		case InboxActionAssign:
			updates["status"] = InboxStatusAssigned
			updates["assigned_at"] = &now
		case InboxActionReassign:
			updates["status"] = InboxStatusAssigned
			updates["assigned_at"] = &now
		case InboxActionRelease:
			updates["status"] = InboxStatusOpen
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
			updates["assigned_at"] = nil
		case InboxActionClose:
			updates["status"] = InboxStatusClosed
			updates["closed_at"] = &now
		case InboxActionReopen:
			updates["status"] = InboxStatusUnread
			updates["closed_at"] = nil
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
		}

		if req.Action == InboxActionAssign || req.Action == InboxActionReassign {
			updates["assigned_to"] = ""
			updates["assigned_to_sop"] = 0
			switch req.ToType {
			case InboxAssignToHuman:
				updates["assigned_to"] = req.ToUserID
				s.addLoad(ctx, req.ToUserID)
			case InboxAssignToSOP:
				updates["assigned_to_sop"] = req.ToSOPID
			case InboxAssignToAI:
				// 暂不绑定具体 ID
			}
		}

		if err := tx.Model(&model.InboxConversation{}).
			Where("id = ?", req.ConversationID).
			Updates(updates).Error; err != nil {
			return err
		}

		hist := &model.InboxAssignment{

			ConversationID:	conv.ID,
			Platform:	conv.Platform,
			AccountID:	conv.AccountID,
			CustomerID:	conv.CustomerID,
			Action:		req.Action,
			FromType:	inferFromType(conv.AssignedTo, conv.AssignedToSOP),
			FromUserID:	conv.AssignedTo,
			ToType:		req.ToType,
			ToUserID:	req.ToUserID,
			ToSOPID:	req.ToSOPID,
			OperatorID:	req.OperatorID,
			Remark:		req.Remark,
		}
		if err := tx.Create(hist).Error; err != nil {
			return err
		}
		history = hist
		return nil
	})
	if err != nil {
		return nil, err
	}
	return history, nil
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
		ConversationID:	conversationID,
		Action:		InboxActionAssign,
		ToType:		InboxAssignToHuman,
		ToUserID:	staff,
		OperatorID:	operatorID,
		Remark:		"auto-assign",
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
		ConversationID:	conversationID,
		Action:		InboxActionAssign,
		ToType:		InboxAssignToHuman,
		ToUserID:	staff,
		OperatorID:	operatorID,
		Remark:		"round-robin",
	})
}

// StaffLoad 客服当前负载
func (s *InboxService) StaffLoad(ctx context.Context, staffUserID string) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	var n int64
	if err := s.db.Model(&model.InboxConversation{}).
		Where("assigned_to = ? AND status IN ?", staffUserID, []string{InboxStatusAssigned, InboxStatusOpen}).
		Count(&n).Error; err != nil {
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
	if s.db == nil {
		return []*model.InboxAssignment{}, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	tx := s.db.Model(&model.InboxAssignment{})
	if conversationID > 0 {
		tx = tx.Where("conversation_id = ?", conversationID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.InboxAssignment
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetStats 收件箱统计
func (s *InboxService) GetStats(ctx context.Context) (*InboxStats, error) {
	if s.db == nil {
		return &InboxStats{ByPlatform: map[string]int64{}, ByAssignedTo: map[string]int64{}}, nil
	}
	stats := &InboxStats{
		ByPlatform:	map[string]int64{},
		ByAssignedTo:	map[string]int64{},
	}
	// 状态分布
	for _, st := range []string{InboxStatusUnread, InboxStatusOpen, InboxStatusAssigned, InboxStatusClosed} {
		var n int64
		if err := s.db.Model(&model.InboxConversation{}).
			Where("status = ?", st).
			Count(&n).Error; err != nil {
			return nil, err
		}
		switch st {
		case InboxStatusUnread:
			stats.Unread = n
		case InboxStatusOpen:
			stats.Open = n
		case InboxStatusAssigned:
			stats.Assigned = n
		case InboxStatusClosed:
			stats.Closed = n
		}
		stats.Total += n
	}

	// 平台分布
	type pc struct {
		Platform	string
		C		int64
	}
	var pcs []pc
	s.db.Model(&model.InboxConversation{}).
		Select("platform AS platform, COUNT(*) AS c").
		Group("platform").Scan(&pcs)
	for _, p := range pcs {
		stats.ByPlatform[p.Platform] = p.C
	}

	// 客服分布
	type ac struct {
		AssignedTo	string
		C		int64
	}
	var acs []ac
	s.db.Model(&model.InboxConversation{}).
		Select("assigned_to, COUNT(*) AS c").
		Where("assigned_to <> '' AND status IN ?", []string{InboxStatusAssigned, InboxStatusOpen}).
		Group("assigned_to").Scan(&acs)
	for _, a := range acs {
		stats.ByAssignedTo[a.AssignedTo] = a.C
	}

	// 超时未响应（最近 30 分钟内客户消息无客服回复）
	threshold := time.Now().Add(-30 * time.Minute)
	var overdue int64
	s.db.Model(&model.InboxConversation{}).
		Where("status IN ? AND last_message_from = ? AND last_message_at <= ?", []string{InboxStatusUnread, InboxStatusOpen, InboxStatusAssigned}, InboxFromCustomer, threshold).
		Count(&overdue)
	stats.OverdueCount = overdue

	return stats, nil
}

// GetMessagesByConversation 拉取会话下的消息（来自 message_hub）
func (s *InboxService) GetMessagesByConversation(ctx context.Context, conversationID uint, page, pageSize int) ([]*model.MessageHub, int64, error) {
	if s.db == nil {
		return []*model.MessageHub{}, 0, nil
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
	tx := s.db.Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND (sender_id = ? OR receiver_id = ?)", conv.Platform, conv.AccountID, conv.CustomerID, conv.CustomerID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.MessageHub
	if err := tx.Order("sent_at ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
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
	type c struct {
		AssignedTo	string
		N		int64
	}
	var counts []c
	s.db.Model(&model.InboxAssignment{}).
		Select("to_user_id AS assigned_to, COUNT(*) AS n").
		Where("to_user_id IN ? AND action = ?", candidates, InboxActionAssign).
		Group("to_user_id").Scan(&counts)

	got := make(map[string]int64, len(candidates))
	for _, c := range counts {
		got[c.AssignedTo] = c.N
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
