package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

var dyMeaningfulRe = regexp.MustCompile(`[\p{L}\p{N}]`)

func (s *WebhookService) mineDouyinGroupLead(ctx context.Context, hub *model.MessageHub, accountID, groupID, groupTitle, fromID, fromName, text string) (newOpportunity bool) {
	if s == nil {
		return false
	}
	s.ensureReposFromDB(ctx)
	if s.clueRepo == nil {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "/") {
		return false
	}
	if !dyMeaningfulRe.MatchString(t) {
		return false
	}

	account := "@" + strings.TrimLeft(fromName, "@")
	if account == "@" || account == "" {
		account = "dy:" + strings.TrimSpace(fromID)
	}
	if account == "dy:" || account == "" {
		return false
	}

	name := strings.TrimSpace(fromName)
	if name == "" {
		name = account
	}

	score, signals, isOpp := DetectDouyinIntent(t)
	chatLabel := groupTitle
	if strings.TrimSpace(chatLabel) == "" {
		chatLabel = "抖音直播间"
	}
	desc := FormatDouyinLeadDesc(chatLabel, t, score, signals, isOpp)

	msgID, convID, oneID := "", "", ""
	if hub != nil {
		msgID = hub.MsgID
		convID = hub.ConversationID
		oneID = hub.SenderID
	}

	existing, err := s.clueRepo.FindByTypeAndAccount(ctx, ClueTypeDouyin, account)
	if err != nil {
		logger.Warnf("[DyLeadMiner] 查询抖音线索失败 account=%s: %v", account, err)
		return false
	}

	if existing == nil {
		clue := &model.Clue{
			SourceID:       groupID,
			Account:        account,
			Type:           ClueTypeDouyin,
			Name:           name,
			Desc:           desc,
			IntentScore:    int64(score),
			IsOpportunity:  boolToInt64(isOpp),
			MessageID:      msgID,
			ConversationID: convID,
			OneID:          oneID,
			IsGroup:        hub != nil && hub.IsGroup,
			GroupID:        groupID,
			GroupName:      groupTitle,
		}
		if cerr := s.clueRepo.Create(ctx, clue); cerr != nil {
			if re, rerr := s.clueRepo.FindByTypeAndAccount(ctx, ClueTypeDouyin, account); rerr == nil && re != nil {
				existing = re
			} else {
				if !isDuplicateKeyError(cerr) {
					logger.Warnf("[DyLeadMiner] 创建抖音线索失败 account=%s: %v", account, cerr)
				}
				return false
			}
		} else {
			logger.Infof("[DyLeadMiner] 新增抖音线索 account=%s 意向分=%d 商机=%v 群=%s", account, score, isOpp, groupTitle)
			newOpportunity = isOpp
			s.recordDouyinLeadScore(ctx, clue, isOpp)
			if isOpp && score >= dyDMOutreachMinScore {
				s.triggerDouyinDMOutreach(ctx, accountID, fromID, groupID, groupTitle, score, text)
			}
			return newOpportunity
		}
	}

	// 若线索已存在但商机状态从 0 → 1（首次判定为商机），也要触发私信
	if existing != nil && existing.IsOpportunity == 0 && isOpp && score >= dyDMOutreachMinScore {
		logger.Infof("[DyLeadMiner] 线索首次升级为商机，触发私信 account=%s", account)
		newOpportunity = true
		s.triggerDouyinDMOutreach(ctx, accountID, fromID, groupID, groupTitle, score, text)
	}

	if int64(score) > existing.IntentScore {
		wasOpp := existing.IsOpportunity != 0
		updates := map[string]any{
			"desc":            desc,
			"source_id":       groupID,
			"intent_score":    int64(score),
			"is_opportunity":  boolToInt64(isOpp),
			"message_id":      msgID,
			"conversation_id": convID,
			"one_id":          oneID,
			"group_id":        groupID,
			"group_name":      groupTitle,
		}
		if uerr := s.clueRepo.UpdateByID(ctx, existing.ID, updates); uerr != nil {
			logger.Warnf("[DyLeadMiner] 更新抖音线索失败 id=%s: %v", existing.ID, uerr)
		} else {
			existing.IntentScore = int64(score)
			existing.IsOpportunity = boolToInt64(isOpp)
			existing.Desc = desc
			existing.SourceID = groupID
			logger.Infof("[DyLeadMiner] 更新抖音线索意向分 account=%s → %d 商机=%v", account, score, isOpp)
			newOpportunity = isOpp && !wasOpp
			// newOpportunity=true 时上面已经触发过私信，避免重复
			if isOpp && score >= dyDMOutreachMinScore && newOpportunity {
				s.triggerDouyinDMOutreach(ctx, accountID, fromID, groupID, groupTitle, score, text)
			}
		}
	}
	s.recordDouyinLeadScore(ctx, existing, isOpp)
	return newOpportunity
}

func (s *WebhookService) triggerDouyinDMOutreach(ctx context.Context, accountID, fromID, groupID, groupTitle string, score int, originalText string) {
	dySvc := NewDouyinIntegrationService(s.db)
	if dySvc == nil {
		return
	}

	convKey := "mtk:dy:dm_sent:" + accountID + ":" + fromID
	if !dySvc.DMOutreachAllowed(ctx, accountID, fromID) {
		logger.Debugf("[DyDM-Outreach] 冷却中，跳过 account=%s user=%s", accountID, fromID)
		return
	}

	dmMsg := BuildDouyinDMWelcome(groupTitle, originalText)
	dmConvID := "dy_dm_" + fromID

	if err := dySvc.SendMessage(ctx, accountID, dmConvID, dmMsg); err != nil {
		logger.Warnf("[DyDM-Outreach] 私信发送失败 account=%s user=%s: %v", accountID, fromID, err)
		return
	}

	logger.Infof("[DyDM-Outreach] 群线索转私信成功 account=%s user=%s score=%d group=%s",
		accountID, fromID, score, groupID)

	s.recordDouyinDMOutreachEvent(ctx, accountID, fromID, groupID, score, dmMsg)
	_ = convKey
}

func (s *WebhookService) recordDouyinDMOutreachEvent(ctx context.Context, accountID, fromID, groupID string, score int, msg string) {
	if s.db == nil {
		return
	}
	defer func() { _ = recover() }()

	hub := &model.MessageHub{
		Platform:       "douyin",
		AccountID:      accountID,
		MsgID:          "dy_dm_outreach_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Direction:      "outbound",
		MsgType:        "text",
		SenderID:       accountID,
		ReceiverID:     fromID,
		Content:        msg,
		ConversationID: "dy_dm_" + fromID,
		IsGroup:        false,
		SentAt:         time.Now(),
		IsAIReply:      true,
		AIAgent:        "lead_outreach",
		Extra: model.JSONMap{
			"scenario":     "group_to_dm",
			"trigger":      "high_intent_lead",
			"intent_score": score,
			"source_group": groupID,
			"channel":      "douyin",
		},
	}
	if err := s.messageHubRepo.Create(ctx, hub); err != nil {
		logger.Warnf("[DyDM-Outreach] 记录私信事件失败: %v", err)
	}
}

func (s *WebhookService) recordDouyinLeadScore(ctx context.Context, clue *model.Clue, isOpp bool) {
	if s == nil || s.db == nil || clue == nil || clue.ID == "" {
		return
	}
	defer func() { _ = recover() }()
	scoreSvc := NewClueScoreServiceWithRepos(
		repository.NewClueScoreRepositoryWithDB(s.db),
		repository.NewClueEngagementRepositoryWithDB(s.db),
		s.clueRepo,
	)
	_ = scoreSvc.RecordEngagement(context.Background(), clue.ID, "group_message", "douyin", map[string]any{"is_opportunity": isOpp})
	_, _ = scoreSvc.ScoreClue(context.Background(), clue)
}

func (s *WebhookService) DouyinLeadMiner() func(ctx context.Context, ev *model.MessageEvent) {
	return func(ctx context.Context, ev *model.MessageEvent) {
		if ev == nil {
			return
		}
		channel := strings.ToLower(strings.TrimSpace(ev.Channel))
		// 2026-08-16 扩展：所有 Bridge 网页渠道统一走抖音关键词打分模型（兼容闲鱼/微博/淘宝等）
		if !isBridgeLeadMiningChannel(channel) {
			return
		}
		if strings.TrimSpace(ev.Content) == "" {
			return
		}
		if ev.SenderType == "agent" || ev.SenderType == "self" {
			return
		}

		accountID := ""
		groupName := ""
		if ev.Extra != nil {
			if v, ok := ev.Extra["account_id"]; ok {
				accountID = toString(v)
			}
			if v, ok := ev.Extra["group_name"]; ok {
				groupName = toString(v)
			}
		}

		hub := &model.MessageHub{
			Platform:       channel,
			AccountID:      accountID,
			MsgID:          ev.EventID,
			Direction:      "inbound",
			MsgType:        ev.MsgType,
			SenderID:       ev.SenderID,
			SenderName:     ev.SenderName,
			ReceiverID:     ev.ReceiverID,
			Content:        ev.Content,
			ConversationID: ev.ConversationID,
			IsGroup:        ev.IsGroup,
			GroupID:        ev.GroupID,
			Extra: model.JSONMap{
				"source":      "bridge",
				"group_name":  groupName,
				"account_id":  accountID,
			},
		}

		s.mineDouyinGroupLead(ctx, hub, accountID, ev.GroupID, groupName, ev.SenderID, ev.SenderName, ev.Content)
	}
}

func isBridgeLeadMiningChannel(channel string) bool {
	switch channel {
	// 2026-08-16 修正：仅保留真实有 Chrome 扩展 + Bridge 协议 + channelgw 注册的 5 个渠道
	case "douyin", "tiktok", "kuaishou", "xiaohongshu", "xianyu":
		return true
	// 微博/淘宝/拼多多/京东/1688/B站等渠道：暂未实现 Chrome 扩展与 Bridge 协议，
	// 不能因为白名单写着"支持"就把消息路由过去——必须先实现完整链路再启用。
	}
	return false
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}