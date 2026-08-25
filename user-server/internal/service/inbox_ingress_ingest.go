package service

import (
	"context"
	"time"
	"unicode"

	"errors"

	"hivemtk-user/internal/model"

	"crypto/sha256"
	"encoding/hex"
	"strings"

	"gorm.io/gorm"
)

type IngressDecision struct {
	Blocked    bool
	IsSelfEcho bool
	IsDup      bool
	Reason     string
}

// resolveSenderKey 解析消息的物理发送者标识（用于去重键的发送者维度）。
//
//	优先级：SenderName > SenderID > ConversationID（兜底，按会话隔离避免跨客户碰撞）> "unknown"。
//	说明：上报数据在前端无法可靠获取发送者名称（自他消息不准确），但平台回传的 SenderID
//	是真实客户/账号的物理标识（可靠）；仅"自/他标签"不可信，故自他由服务端 DB 回查判定。
func (s *InboxIngressService) resolveSenderKey(event *model.MessageEvent) string {
	if event.SenderName != "" {
		return event.SenderName
	}
	if event.SenderID != "" {
		return event.SenderID
	}
	if event.ConversationID != "" {
		return event.ConversationID
	}
	return "unknown"
}

// resolveAccountID 从事件 Extra 中取账号（平台身份）。
func resolveAccountID(event *model.MessageEvent) string {
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// senderKeyForDedup 计算消息的去重发送者键（渠道+发送者+内容 三元组的"发送者"维度）。
//
//	核心：自他判定服务端权威，不信任前端 sender_type。
//	- 平台自己发出的消息（sender_type=self/agent，即 AI/人工客服）一律归一为账号(platform 身份)，
//	  与出站(outbound)落库时填入的 senderKey=accountID 保持一致 → 回显时哈希匹配被识别为"自己消息"。
//	- 其他（真实客户/上报不可信消息）使用物理发送者标识（SenderName/SenderID），本身即区分不同客户。
func (s *InboxIngressService) senderKeyForDedup(event *model.MessageEvent) string {
	sk := s.resolveSenderKey(event)
	if event.SenderType == "self" || event.SenderType == "agent" {
		if acc := resolveAccountID(event); acc != "" {
			sk = acc
		}
	}
	return sk
}

// senderDefinitelyDiffers 服务端权威自他判定：仅当入站事件与命中的 outbound 行
// 双方都携带可靠物理发送者标识（SenderID）且不相等时返回 true（即"确定不是自己回显"）。
//
// 设计约束：
//   - self/agent 事件一律视为平台自身消息（不豁免，保持回环拦截）
//   - 任一侧 SenderID 缺失时返回 false（信息不足，保守按回显处理，
//     兼容 bridge patrol 上报 sender_name 为对方昵称而 outbound 落库 sender_name 为空的历史场景）
func (s *InboxIngressService) senderDefinitelyDiffers(event *model.MessageEvent, ob *model.MessageHub) bool {
	if event == nil || ob == nil {
		return false
	}
	switch event.SenderType {
	case "self", "agent":
		return false
	}
	inSender := strings.TrimSpace(event.SenderID)
	outSender := strings.TrimSpace(ob.SenderID)
	if inSender == "" || outSender == "" {
		return false
	}
	return inSender != outSender
}

// interceptInbound 统一收件中间件：在消息落库/触发 AI 之前，依据「渠道+发送者+内容」唯一去重依据
// 做服务端权威拦截，避免无效/重复/回环消息穿透业务层。
//
// 设计要点（对照用户诉求）：
//  1. 去重依据 = 渠道 + 发送者名称 + 消息内容（ContentHashWithSender），前端后端共用同一算法。
//  2. 自/他判定依赖数据库检查消息哈希，而非前端不可信的 sender_type：
//     平台自己发出的消息必然先以 direction='outbound' 落库（AI/人工回复，SenderID=账号）。
//     故"当前上报命中同 dedup_hash 的 outbound 行"即判定为自己消息回显（回环），拦截。
//  3. 关键修复：去重键含发送者 → 「客户复述了 AI 的原话」因发送者不同哈希不同，
//     不再被旧逻辑（仅按 platform+content 匹配 outbound）误判为回显而丢失客户消息。
//  4. AI 返回的才是真正自己消息；前端上报的可能是自己也可能是他人 → 经 DB 哈希回查分别处理。
//
// 返回值：Blocked=true 时调用方应直接 ack 跳过，不落库、不触发 AI。
func (s *InboxIngressService) interceptInbound(ctx context.Context, event *model.MessageEvent) (*IngressDecision, error) {
	if s.hubRepo == nil {

		return &IngressDecision{}, nil
	}
	content := strings.TrimSpace(event.Content)
	if content == "" || event.Channel == "" {

		return &IngressDecision{}, nil
	}

	if ob, oerr := s.hubRepo.GetOutboundByPlatformSenderContentConv(ctx, event.Channel, event.SenderName, content, event.ConversationID); oerr == nil && ob != nil && !s.senderDefinitelyDiffers(event, ob) {
		return &IngressDecision{Blocked: true, IsSelfEcho: true, Reason: "self-echo(matched outbound by platform+sender_name+content)"}, nil
	}

	// 兜底 3：本账号同会话近期 outbound 归一化内容命中。
	// DOM 抓取回显常带空白/零宽字符变体（换行→空格、U+200B/U+3000 注入），
	// 精确匹配与包含匹配均漏判 → 按"剥除全部空白+格式字符后相等"兜底拦截。
	// 窗口 2h：既覆盖 patrol 轮询周期，又不误杀跨日重复话术（回归用例 3h 前的 outbound 不拦）。
	if event.ConversationID != "" && s.hubRepo != nil {
		rows, rerr := s.hubRepo.ListRecentOutboundInConv(ctx, event.Channel, resolveAccountID(event), event.ConversationID, time.Now().Add(-2*time.Hour), 20)
		if rerr == nil && len(rows) > 0 {
			norm := normalizeEchoText(content)
			if norm != "" {
				for i := range rows {
					if normalizeEchoText(rows[i].Content) == norm {
						return &IngressDecision{Blocked: true, IsSelfEcho: true, Reason: "self-echo(recent outbound normalized match)"}, nil
					}
				}
			}
		}
	}

	if s.cache != nil {
		dedupHash := ContentHashWithSender(event.Channel, s.senderKeyForDedup(event), content)
		dupKey := InboxSenderContentDedupKey + dedupHash
		if ok, derr := s.cache.SetNX(ctx, dupKey, "1", InboxContentDedupTTL); derr == nil && !ok {
			return &IngressDecision{Blocked: true, IsDup: true, Reason: "duplicate(channel+sender+content) within window"}, nil
		}
	}

	return &IngressDecision{}, nil
}

// isDuplicateKey 判断是否为唯一键冲突（Postgres: duplicate key value on ...）。
// 用于消息落库幂等：同一 MsgID（event_id）重发/重扫时视为已落库，不报错。
//
// 2026-08-17 补充：并发竞态下，两个请求都通过 GetByMsgID 预检查（都看到 0 行），
// 然后都进入事务 INSERT。第二个请求的 INSERT 报 duplicate key，但 PostgreSQL 已把
// 事务标记为 aborted → GORM 在 commit 阶段抛出 "commit unexpectedly resulted in rollback"。
// 这个包装错误不含 "duplicate key" 字样，需额外识别，否则整批 batch 误判为持久化失败。
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "commit unexpectedly resulted in rollback") ||
		errors.Is(err, gorm.ErrDuplicatedKey)
}

// contentHashOf 计算消息内容的 SHA-256 hash（用于 5 分钟内内容去重）。
// 返回前 16 字符的十六进制字符串（128 位足够区分重复内容，key 不会过长）。
//
// 2026-08-05 架构重构：Bridge 端不再做内容指纹去重，由服务端统一判断。
func contentHashOf(content string) string {
	if content == "" {
		return ""
	}
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}

// groupNameOf 从 MessageEvent 提取群名：优先 GroupID 对应的 GroupName 字段（事件模型
// 无 GroupName 时回退 Extra 冗余），保证群聊 AI 编排能拿到群名。
func groupNameOf(event *model.MessageEvent) string {
	if event == nil {
		return ""
	}
	if event.Extra != nil {
		if v, ok := event.Extra["group_name"]; ok {
			if s, _ := v.(string); s != "" {
				return s
			}
		}
	}
	return ""
}


// normalizeEchoText 回环比对归一化：剥除所有空白（含 U+3000 全角空格）
// 与格式字符（含 U+200B 零宽空格等 Cf 类），仅保留可见语义字符。
func normalizeEchoText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
