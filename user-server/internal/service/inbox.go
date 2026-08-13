package service

import (
	"context"

	"errors"

	"fmt"

	"strings"

	"sync"

	"time"

	"hivemtk-user/internal/model"

	dbUtil "hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/utils/logger"
	"sort"

	"gorm.io/gorm"
)

var (
	ErrInboxEmptyMerchant = errors.New("user_id is required")

	ErrInboxInvalidPlatform = errors.New("invalid platform")

	ErrInboxInvalidAccount = errors.New("invalid account_id")

	ErrInboxInvalidCustomer = errors.New("invalid customer_id")

	ErrInboxInvalidAssignTo = errors.New("invalid assign_to")

	ErrInboxInvalidAction = errors.New("invalid assignment action")

	ErrInboxConversationExist = errors.New("conversation already exists for this account/customer")

	ErrInboxConversationMissing = errors.New("conversation not found")

	ErrInboxInvalidStatus = errors.New("invalid conversation status")

	ErrInboxRepoNotReady = errors.New("inbox repository not ready")
)

const (
	InboxStatusUnread = "unread"

	InboxStatusOpen = "open"

	InboxStatusAssigned = "assigned"

	InboxStatusClosed = "closed"
)

const InboxOverdueThreshold = 30 * time.Minute

const (
	ReconcileModeUnread = "unread" // 以 message_hub 最后一条消息为事实源，重算未读/状态（修正历史数据）

	ReconcileModeOverdue = "overdue" // 超时未响应：转在线人工坐席处理（否则默认 AI 处理）

	ReconcileModeBackfill = "backfill" // 回填：修正 NULL/空 conversation_id 脏数据 + 为历史会话补建 inbox_conversations（消除 sync_gap 缺口）

)

const (
	InboxActionAssign = "assign"

	InboxActionReassign = "reassign"

	InboxActionRelease = "release"

	InboxActionClose = "close"

	InboxActionReopen = "reopen"
)

const (
	InboxAssignToHuman = "human"

	InboxAssignToSOP = "sop"

	InboxAssignToAI = "ai"
)

const (
	InboxFromCustomer = "customer"

	InboxFromStaff = "staff"

	InboxFromAI = "ai"
)

func inboxCustomerID(msg *model.MessageHub) string {
	if msg == nil {
		return ""
	}
	// 群聊：以会话本身作为归属实体
	if msg.IsGroup && msg.ConversationID != "" {
		return msg.ConversationID
	}
	// 群聊 / 聚合会话：sender_id（入站）或 receiver_id（出站）被写成
	// "conversation_id <时间后缀>"，并非稳定参与方 ID → 归一为 conversation_id
	if msg.ConversationID != "" {
		if msg.SenderID != "" && strings.HasPrefix(msg.SenderID, msg.ConversationID+" ") {
			return msg.ConversationID
		}
		if msg.ReceiverID != "" && strings.HasPrefix(msg.ReceiverID, msg.ConversationID+" ") {
			return msg.ConversationID
		}
	}
	// 常规：出站取 receiver，入站取 sender
	if msg.Direction == "outbound" {
		if msg.ReceiverID != "" {
			return msg.ReceiverID
		}
		return msg.SenderID
	}
	return msg.SenderID
}

func inboxCustomerName(msg *model.MessageHub) string {
	name := msg.SenderName
	if name == "" && inboxCustomerID(msg) == msg.ConversationID {
		name = cleanConversationTitle(msg.ConversationID)
	}
	return name
}

func cleanConversationTitle(convID string) string {
	if convID == "" {
		return ""
	}
	if strings.HasPrefix(convID, "conv:") {
		return strings.TrimPrefix(convID, "conv:")
	}
	return convID
}

const InboxDefaultStaffLoadLimit = 30

type InboxService struct {
	inboxRepo      *repository.InboxConversationRepository
	assignmentRepo *repository.InboxAssignmentRepository
	hubRepo        *repository.MessageHubRepository
	sessionMsgRepo *repository.SessionMessageRepository
	mu             sync.RWMutex
	staffLoadCache map[string]int // staffUserID -> 当前承接数（缓存）
}

func NewInboxService() *InboxService {
	return NewInboxServiceWithDB(dbUtil.GetDB())
}

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

type InboxAssignRequest struct {
	ConversationID uint
	Action         string // assign/reassign/release/close/reopen
	ToType         string // human/sop/ai
	ToUserID       string
	ToSOPID        uint
	OperatorID     string
	Remark         string
}

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

type ConversationKey struct {
	Platform   string
	AccountID  string
	CustomerID string
}

func (k ConversationKey) Key(ctx context.Context) string {
	return k.Platform + "|" + k.AccountID + "|" + k.CustomerID
}

func (s *InboxService) UpsertFromMessage(ctx context.Context, msg *model.MessageHub) error {
	if msg == nil {
		return nil
	}
	_, err := s.UpsertFromHubMessage(ctx, msg)
	return err
}

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
	// 取出会话（归属键与 monitor sync_gap 判定一致）
	cid := inboxCustomerID(msg)
	conv, err := s.inboxRepo.FindByPlatformAccountCustomer(ctx, msg.Platform, msg.AccountID, cid)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *InboxService) UpsertFromHubMessageTx(ctx context.Context, msg *model.MessageHub, hubRepo *repository.MessageHubRepository) (*model.InboxConversation, error) {
	if s.inboxRepo == nil || msg == nil {
		return nil, nil
	}
	if hubRepo == nil {
		// 兜底：无 hubRepo 时退化为非事务版本
		return s.UpsertFromHubMessage(ctx, msg)
	}
	// 复用 upsertInternal 的 input 构造逻辑
	if msg.Platform == "" || msg.AccountID == "" {
		return nil, ErrInboxEmptyMerchant
	}
	customerID := inboxCustomerID(msg)
	if customerID == "" {
		return nil, ErrInboxInvalidCustomer
	}
	now := time.Now()
	lastMsgAt := msg.SentAt
	if lastMsgAt.IsZero() {
		lastMsgAt = now
	}
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
		CustomerName:       inboxCustomerName(msg),
		ConversationID:     msg.ConversationID,
		LastMessageID:      msg.ID,
		LastMessagePreview: preview,
		LastMessageAt:      lastMsgAt,
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

func (s *InboxService) upsertInternal(ctx context.Context, msg *model.MessageHub) error {
	if s.inboxRepo == nil || msg == nil {
		return nil
	}
	if msg.Platform == "" || msg.AccountID == "" {
		return ErrInboxEmptyMerchant
	}
	// 默认以 sender 作为 customerId。群聊/聚合消息统一以 conversation_id 归属（见 inboxCustomerID）。
	customerID := inboxCustomerID(msg)
	if customerID == "" {
		return ErrInboxInvalidCustomer
	}

	now := time.Now()
	lastMsgAt := msg.SentAt
	if lastMsgAt.IsZero() {
		lastMsgAt = now
	}
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
		CustomerName:       inboxCustomerName(msg),
		ConversationID:     msg.ConversationID,
		LastMessageID:      msg.ID,
		LastMessagePreview: preview,
		LastMessageAt:      lastMsgAt,
		LastMessageFrom:    from,
	})
}

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

func (s *InboxService) MarkRead(ctx context.Context, conversationID uint) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.MarkRead(ctx, conversationID)
}

func (s *InboxService) Pin(ctx context.Context, conversationID uint, pinned bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "pinned", pinned)
}

func (s *InboxService) Star(ctx context.Context, conversationID uint, starred bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "starred", starred)
}

func (s *InboxService) Mute(ctx context.Context, conversationID uint, muted bool) error {
	if s.inboxRepo == nil {
		return nil
	}
	return s.inboxRepo.UpdateField(ctx, conversationID, "muted", muted)
}

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

func (s *InboxService) ListAssignments(ctx context.Context, conversationID uint, page, pageSize int) ([]*model.InboxAssignment, int64, error) {
	if s.assignmentRepo == nil {
		return []*model.InboxAssignment{}, 0, nil
	}
	return s.assignmentRepo.ListByConversationID(ctx, conversationID, page, pageSize)
}

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

type ReconcileResult struct {
	Mode                 string `json:"mode"`
	UnreadReconciled     int64  `json:"unread_reconciled"`
	OverdueFound         int    `json:"overdue_found"`
	OverdueAssigned      int    `json:"overdue_assigned"`
	AssignedTo           string `json:"assigned_to,omitempty"`
	FixedNullConv        int64  `json:"fixed_null_conv_id"`     // backfill：修正的 NULL/空 conversation_id 行数
	Backfilled           int64  `json:"backfilled"`             // backfill：补建的 inbox_conversations 会话数
	NormalizedConv       int64  `json:"normalized_conv"`        // backfill：归一的时间戳污染 conversation_id 数
	PollutedInboxDeleted int64  `json:"polluted_inbox_deleted"` // backfill：删除的时间戳污染孤儿收件箱行数
	SyncGapFixed         int64  `json:"sync_gap_fixed"`         // backfill：修复的 sync_gap 会话数
	Message              string `json:"message"`
}

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

	// 步骤 2：归一被时间戳污染的 conversation_id（如 "conv:群标题 29分钟前" → "conv:群标题"）。
	// 同一会话被拆成多条带时间戳变体的 message_hub 记录因此合并到规范会话键，从根上消除碎片化；
	// message_trace 同步归一，避免链路追踪树按旧键分裂。
	normN, err := s.hubRepo.NormalizePollutedConversationIDs(ctx)
	if err != nil {
		return nil, err
	}
	res.NormalizedConv = normN
	if normT, err := s.hubRepo.NormalizePollutedTraceConversationIDs(ctx); err != nil {
		logger.Warnf("reconcile backfill: 归一 message_trace conversation_id 失败: %v", err)
	} else if normT > 0 {
		logger.Infof("reconcile backfill: 归一 message_trace conversation_id %d 条", normT)
		res.NormalizedConv += normT
	}

	// 步骤 3：清理归一后失效的收件箱孤儿行。
	// 3a) 删带空格后缀的污染行（防御性，当前数据已无空格行）。
	delN, err := s.inboxRepo.DeletePollutedInboxRows(ctx)
	if err != nil {
		return nil, err
	}
	// 3b) 删不再被 message_hub 引用的 conv: 短键孤儿行（早期 split_part 错切合并残留，
	// 如 "conv:AI"——归一后 message_hub 已无此键，须清除以免收件箱出现陈旧/重复会话）。
	orphanN, err := s.inboxRepo.DeleteOrphanConvInboxRows(ctx)
	if err != nil {
		return nil, err
	}
	res.PollutedInboxDeleted = delN + orphanN

	// 步骤 4：回填缺失的 inbox_conversations 历史会话
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

	// 步骤 5：全量修复 sync_gap（与 monitor 判定一致的规范客户键）。
	gapConvs, err := s.hubRepo.FindSyncGapConversations(ctx, time.Now().AddDate(0, 0, -90))
	if err != nil {
		return nil, err
	}
	for _, gc := range gapConvs {
		if gc.CustomerID == "" {
			continue
		}
		if err := s.reconcileSyncGapConversation(ctx, gc.Platform, gc.AccountID, gc.ConversationID, gc.CustomerID); err != nil {
			logger.Warnf("reconcile backfill: 会话 %s 修复失败（已跳过）: %v", gc.ConversationID, err)
			continue
		}
		res.SyncGapFixed++
	}

	res.Message = fmt.Sprintf("已修正 %d 条 NULL/空 conversation_id，归一 %d 条污染会话键，删除 %d 条污染收件箱行，补建 %d 个收件箱会话，修复 %d 个 sync_gap 会话",
		res.FixedNullConv, res.NormalizedConv, res.PollutedInboxDeleted, res.Backfilled, res.SyncGapFixed)
	logger.Infof("reconcile backfill 完成: %s", res.Message)
	return res, nil
}

// reconcileSyncGapConversation 把单个 sync_gap 会话物化为按规范 customer_id 归属的
// 收件箱行，并清理同一会话下 customer_id 不一致的孤儿行。customerID 来自 monitor 的
// 规范客户键判定（与 inboxCustomerID 一致），避免依赖单条最新消息推导导致错键。
func (s *InboxService) reconcileSyncGapConversation(ctx context.Context, platform, accountID, conversationID, customerID string) error {
	latest, err := s.hubRepo.FindLatestByConversation(ctx, conversationID)
	if err != nil || latest == nil {
		// 无代表消息时仍写入一行空预览记录，确保 monitor 不再报缺口
		uerr := s.inboxRepo.UpsertFromMessage(ctx, repository.UpsertFromMessageInput{
			Platform:       platform,
			AccountID:      accountID,
			CustomerID:     customerID,
			CustomerName:   cleanConversationTitle(conversationID),
			ConversationID: conversationID,
			LastMessageAt:  time.Now(),
		})
		return uerr
	}
	from := InboxFromCustomer
	if latest.Direction == "outbound" {
		if latest.IsAIReply {
			from = InboxFromAI
		} else {
			from = InboxFromStaff
		}
	}
	customerName := latest.SenderName
	if customerID == conversationID {
		customerName = cleanConversationTitle(conversationID)
	} else if customerName == "" {
		customerName = latest.ReceiverName
	}
	input := repository.UpsertFromMessageInput{
		Platform:           platform,
		AccountID:          accountID,
		CustomerID:         customerID,
		CustomerName:       customerName,
		ConversationID:     conversationID,
		LastMessageID:      latest.ID,
		LastMessagePreview: latest.Content,
		LastMessageAt:      latest.CreatedAt,
		LastMessageFrom:    from,
	}
	if err := s.inboxRepo.UpsertFromMessage(ctx, input); err != nil {
		return err
	}
	_, err = s.inboxRepo.DeleteOrphanInboxByConversation(ctx, platform, accountID, conversationID, customerID)
	return err
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
