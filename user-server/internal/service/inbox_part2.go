// 拆分自 inbox.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"sort"
	"strings"
	"time"
)

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
