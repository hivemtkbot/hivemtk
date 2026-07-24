// Package service - Telegram 群发言 → 销售线索/商机 自动挖掘
//
// 业务背景：
//
//	Telegram 群里每个真实发言者都是潜在客户——发言即诉求，诉求即线索，线索即商机。
//	因此不能把 AI 触发收敛为"仅回复机器人 / @机器人"，而应把「群消息」当作线索来源全量捕获。
//
// 设计要点（与 AI 自动回复解耦，互不影响）：
//  1. 静默运行：只写线索库（clues）+ 线索评分（clue_scores），不向群里发任何消息，因此绝不刷屏。
//  2. 去重：按 (Type=Telegram, account) 去重，每个独立发言者仅生成一条线索；
//     后续更高意向的发言会「增量更新」该线索的意向分（取最高分），不会重复建条。
//  3. 意向识别：中英双语关键词 + 联系方式信号打分（0-100），score>=40 记为「商机」。
//  4. best-effort：任何 DB / 评分异常都不影响入站主链路（消息中台 / 收件箱 / AI 回复）。
package service

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ClueTypeTelegram 线索来源类型：Telegram
// 与 clue.go 的 clueTypeMap、clue_score.go 的 scoreChannel(case 4) 保持一致。
const ClueTypeTelegram int64 = 4

// tgLeadOpportunityThreshold 意向分达到该值即判定为「商机」
const tgLeadOpportunityThreshold = 40

// tgMeaningfulRe 至少包含一个字母/数字/文字，用于过滤纯表情、纯标点的无效发言
var tgMeaningfulRe = regexp.MustCompile(`[\p{L}\p{N}]`)

// 联系方式信号（出现即视为强购买/合作意向）
var (
	tgEmailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	tgPhoneRe = regexp.MustCompile(`\+?\d[\d\-\s]{6,}\d`)
	tgLinkRe  = regexp.MustCompile(`(?i)(t\.me/|wa\.me/|https?://|whatsapp|wechat|微信|vx[:：]?|加我|私聊)`)
)

// tgHighIntentKeywords 高意向关键词（每命中一次 +25），覆盖中英双语典型购买/合作信号
var tgHighIntentKeywords = []string{
	// 中文
	"多少钱", "价格", "报价", "怎么买", "购买", "下单", "批发", "代理", "加盟",
	"求购", "现货", "有货", "联系方式", "合作", "预算", "采购", "招商",
	// 英文
	"how much", "price", "cost", "quote", "quotation", "buy", "purchase",
	"order", "wholesale", "budget", "procurement", "distributor", "reseller",
}

// tgMediumIntentKeywords 中意向关键词（每命中一次 +12）
var tgMediumIntentKeywords = []string{
	// 中文
	"咨询", "了解", "需要", "想要", "请问", "怎么卖", "优惠", "折扣", "试用",
	"方案", "效果", "质量", "发货", "物流", "功能", "多少",
	// 英文
	"interested", "need", "want", "looking for", "how to", "discount",
	"trial", "demo", "solution", "feature", "quality", "shipping",
}

// DetectTelegramIntent 对一条群发言做购买/合作意向打分。
// 返回：score(0-100)、命中的信号列表、是否达到「商机」阈值。
// 纯函数，便于单测。
func DetectTelegramIntent(text string) (score int, signals []string, isOpportunity bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return 0, nil, false
	}
	lower := strings.ToLower(t)
	score = 8 // 任意有效发言的基础意向分（发言即潜在线索）
	seen := map[string]bool{}
	add := func(sig string, w int) {
		if seen[sig] {
			return
		}
		seen[sig] = true
		signals = append(signals, sig)
		score += w
	}
	for _, kw := range tgHighIntentKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			add(kw, 25)
		}
	}
	for _, kw := range tgMediumIntentKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			add(kw, 12)
		}
	}
	if tgEmailRe.MatchString(t) || tgLinkRe.MatchString(t) || tgPhoneRe.MatchString(t) {
		// 出现/索要联系方式（电话/邮箱/微信/wa.me 等）是最强的购买/合作信号，直接推过商机阈值
		add("联系方式", 35)
	}
	if score > 100 {
		score = 100
	}
	return score, signals, score >= tgLeadOpportunityThreshold
}

// formatTelegramLeadDesc 生成线索描述，首部内嵌 [意向分:NN] 供增量更新时解析取最高分。
func formatTelegramLeadDesc(groupTitle, snippet string, score int, signals []string, isOpportunity bool) string {
	tag := "群发言线索"
	if isOpportunity {
		tag = "群发言商机"
	}
	g := strings.TrimSpace(groupTitle)
	if g == "" {
		g = "未命名群组"
	}
	sig := "无"
	if len(signals) > 0 {
		sig = strings.Join(signals, ",")
	}
	snippet = trimRunes(strings.TrimSpace(snippet), 80)
	var b strings.Builder
	b.WriteString("[意向分:")
	b.WriteString(strconv.Itoa(score))
	b.WriteString("] [TG] ")
	b.WriteString(tag)
	b.WriteString(" | 群「")
	b.WriteString(g)
	b.WriteString("」 | 信号:")
	b.WriteString(sig)
	b.WriteString(" | 最近:「")
	b.WriteString(snippet)
	b.WriteString("」")
	return b.String()
}

// trimRunes 按「字符数」而非字节数截断，避免中文被截半
func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// isDuplicateKeyError 判断是否为数据库唯一/主键冲突（并发去重场景），
// 涵盖 Postgres(pq 23505 / "duplicate key")与中文提示，避免误报为致命错误。
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "重复")
}

// telegramLeadAccountKey 生成去重键：优先 @username，否则回退 tg:<数字ID>
func telegramLeadAccountKey(username, fromID string) string {
	username = strings.TrimSpace(username)
	if username != "" {
		return "@" + strings.TrimPrefix(username, "@")
	}
	fromID = strings.TrimSpace(fromID)
	if fromID != "" && fromID != "0" {
		return "tg:" + fromID
	}
	return ""
}

// mineTelegramGroupLead 将一条 TG 群发言/私聊真人发言挖掘为销售线索/商机。
// 幂等去重 + 意向分增量更新；全过程 best-effort，绝不影响入站主链路。
// hub 为对应的消息中台记录，用于把线索关联到具体会话（销售可直接跳转到聊天上下文）。
//
// 返回值 newOpportunity：本次挖掘是否让该发言者「新晋为商机」（首次达到阈值，或意向分跨过
// 阈值由非商机→商机）。用于上层在群里做「发现线索即主动触达」的精准触发（仅新晋时发一次，避免刷屏）。
func (s *WebhookService) mineTelegramGroupLead(ctx context.Context, hub *model.MessageHub, groupID, groupTitle, fromID, username, fromName, text string) (newOpportunity bool) {
	if s == nil {
		return false
	}
	s.ensureReposFromDB(ctx)
	if s.clueRepo == nil {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "/") { // 空消息 / 命令(含 /callback) 不作为线索
		return false
	}
	if !tgMeaningfulRe.MatchString(t) { // 纯表情 / 纯标点 过滤
		return false
	}
	account := telegramLeadAccountKey(username, fromID)
	if account == "" {
		return false
	}
	name := strings.TrimSpace(fromName)
	if name == "" {
		name = strings.TrimSpace(username)
	}
	if name == "" {
		name = account
	}

	score, signals, isOpp := DetectTelegramIntent(t)
	chatLabel := groupTitle
	if strings.TrimSpace(chatLabel) == "" {
		chatLabel = "私聊"
	}
	desc := formatTelegramLeadDesc(chatLabel, t, score, signals, isOpp)

	// 会话关联字段：让销售在CRM里一键回到该线索的聊天上下文
	msgID, convID, oneID := "", "", ""
	if hub != nil {
		msgID = hub.MsgID
		convID = hub.ConversationID
		oneID = hub.SenderID // 入站消息的 sender 即统一客户（OneID）在 TG 侧的标识
	}

	existing, err := s.clueRepo.FindByTypeAndAccount(ctx, ClueTypeTelegram, account)
	if err != nil {
		logger.Warnf("[LeadMiner] 查询 TG 群线索失败 account=%s: %v", account, err)
		return false
	}

	if existing == nil {
		clue := &model.Clue{
			SourceID:       groupID,
			Account:        account,
			Type:           ClueTypeTelegram,
			Name:           name,
			Desc:           desc,
			IntentScore:    int64(score),
			IsOpportunity:  boolToInt64(isOpp),
			MessageID:      msgID,
			ConversationID: convID,
			OneID:          oneID,
		}
		if cerr := s.clueRepo.Create(ctx, clue); cerr != nil {
			// 并发下另一条 goroutine 可能已抢先插入（唯一键冲突），或任意写入异常：
			// 重新查询，若已存在则按「已存在」逻辑增量更新，保证最终一致、不丢线索。
			if re, rerr := s.clueRepo.FindByTypeAndAccount(ctx, ClueTypeTelegram, account); rerr == nil && re != nil {
				existing = re
			} else {
				if !isDuplicateKeyError(cerr) {
					logger.Warnf("[LeadMiner] 创建 TG 线索失败 account=%s: %v", account, cerr)
				}
				return false
			}
		} else {
			logger.Infof("[LeadMiner] 新增 TG 线索 account=%s 意向分=%d 商机=%v 群=%s", account, score, isOpp, groupTitle)
			newOpportunity = isOpp // 新线索即达到商机阈值 → 新晋商机
			s.recordTelegramLeadScore(ctx, clue, isOpp)
			return newOpportunity
		}
	}

	// 已存在（初次查询命中，或并发冲突后回查命中）：仅当本条意向分更高时增量更新。
	// 意向分以结构化列 intent_score 为准（不再依赖解析 Desc 文本，避免脆弱的正则往返）。
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
		}
		if uerr := s.clueRepo.UpdateByID(ctx, existing.ID, updates); uerr != nil {
			logger.Warnf("[LeadMiner] 更新 TG 线索失败 id=%s: %v", existing.ID, uerr)
		} else {
			existing.IntentScore = int64(score)
			existing.IsOpportunity = boolToInt64(isOpp)
			existing.Desc = desc
			existing.SourceID = groupID
			logger.Infof("[LeadMiner] 更新 TG 线索意向分 account=%s → %d 商机=%v", account, score, isOpp)
			// 仅当本次跨过阈值（非商机 → 商机）时记为「新晋商机」，已为商机的更高分不算新晋
			newOpportunity = isOpp && !wasOpp
		}
	}
	s.recordTelegramLeadScore(ctx, existing, isOpp)
	return newOpportunity
}

// boolToInt64 把布尔转为 0/1，用于 is_opportunity 等标记列。
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// recordTelegramLeadScore 记录一次群互动事件并重算线索评分（best-effort，缺表/异常均不影响主链路）。
func (s *WebhookService) recordTelegramLeadScore(ctx context.Context, clue *model.Clue, isOpp bool) {
	if s == nil || s.db == nil || clue == nil || clue.ID == "" {
		return
	}
	defer func() { _ = recover() }()
	scoreSvc := NewClueScoreServiceWithRepos(
		repository.NewClueScoreRepositoryWithDB(s.db),
		repository.NewClueEngagementRepositoryWithDB(s.db),
		s.clueRepo,
	)
	_ = scoreSvc.RecordEngagement(context.Background(), clue.ID, "group_message", "telegram", map[string]any{"is_opportunity": isOpp})
	_, _ = scoreSvc.ScoreClue(context.Background(), clue)
}
