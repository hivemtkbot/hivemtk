package bridge

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"marketing/internal/model"
)

// =============================================================
// bridge 模块共享辅助函数（与传输层无关：HTTP 模式专属工具，不依赖 WebSocket）
//
// 2026-08-05 架构重构：bridge 模块彻底移除 WebSocket 长连接，
// 原 handler.go 中以下内容仍被 HTTP 端使用，迁移到本文件：
//   - OwnershipChecker：账号归属校验（HTTP ingest 端点使用）
//   - maskTokenBridge / itoa / describeUpstreamQuery：日志脱敏工具
//   - historyItemToEvent / toMessageEvent / orDefault：消息事件映射
//   - maxReplyContentBytes：单条 AI 回复最大字节数（reach_adapter 截断使用）
//
// 与 WebSocket 强耦合的部分（bridgeCheckOrigin / splitAndTrim / trimSpaces /
// bridgeTestAutoReply / WarnOnTestMode / truncateForLog / truncateForLogBytes /
// BridgeClient / BridgeHub / BridgeWSHandler / HandleWebSocket / upgraderBridge 等）
// 全部删除。
// =============================================================

// 单条 AI 回复最大字节数（防止 XSS payload 巨大 + 平台限制）
// 与前端 constants.SECURITY.maxReplyContentBytes 严格对齐
const maxReplyContentBytes = 4 * 1024

// OwnershipChecker 账号归属校验回调：
//   - 输入：uid (JWT 解析的 user_id), channel, accountID
//   - 输出：true 表示当前 user 拥有该 bridge 账号
//   - 由 service 层在初始化时注入（避免 bridge → service 的反向依赖）
type OwnershipChecker func(ctx context.Context, uid uint, channel, accountID string) (bool, error)

// GlobalOwnershipChecker 全局归属校验（service 层在 init 时注入）
var GlobalOwnershipChecker OwnershipChecker

// RegisterOwnershipChecker 注册账号归属校验回调
func RegisterOwnershipChecker(fn OwnershipChecker) {
	GlobalOwnershipChecker = fn
}

// maskTokenBridge token 脱敏：避免完整 JWT 写入日志（私域部署仍按隐私基线）。
// 与扩展端 bridge-client.js describeUpstreamParams 行为对齐：保留前 4 位 + 总长度。
// token 为空时返回空串。
//
// 实现要点：按 rune 切片而非字节切片，避免多字节字符（如中文）被切断产生乱码。
// 为保持与扩展端"保留前 4 位"语义一致，rune 数 <= 4 时整段保留（与字节版一致）。
func maskTokenBridge(token string) string {
	if token == "" {
		return ""
	}
	runes := []rune(token)
	if len(runes) <= 4 {
		return token + "***(" + itoa(len(token)) + " chars)"
	}
	return string(runes[:4]) + "***(" + itoa(len(token)) + " chars)"
}

// itoaLen 简单整数转字符串（避免引入 strconv 依赖膨胀）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// describeUpstreamQuery 把 query 解析为 map（token 字段脱敏）。
// 用于日志展示，不影响真实请求处理。
// 与扩展端 bridge-client.js describeUpstreamParams 输出结构对齐。
func describeUpstreamQuery(values url.Values) map[string]string {
	out := make(map[string]string, len(values))
	for k, vs := range values {
		v := ""
		if len(vs) > 0 {
			v = vs[0]
		}
		if k == "token" {
			out[k] = maskTokenBridge(v)
		} else {
			out[k] = v
		}
	}
	return out
}

// historyItemToEvent 把会话级 history 帧中的单轮（HistoryItem）映射为 model.MessageEvent。
// 会话元数据（channel/account/conversation/群）取自帧顶层的 message，轮次字段取自 item。
//
// HTTP-only 模式（2026-08-05 之后）共用本函数：HTTP ingest 端点收到 history 帧时，
// 同样需要 historyItemToEvent 把每个 HistoryItem 落库。
func historyItemToEvent(m *UnifiedMessage, it *HistoryItem) *model.MessageEvent {
	ch := ToBridgeChannel(m.Channel)
	ts := time.UnixMilli(it.Timestamp)
	if it.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        it.EventID,
		SessionID:      ch + ":" + m.AccountID + ":" + m.ConversationID,
		Channel:        ch,
		SenderID:       it.SenderID,
		SenderName:     it.SenderName,
		ReceiverID:     orDefault(it.ReceiverID, m.ReceiverID),
		MsgType:        it.MsgType,
		Content:        it.Content,
		MediaURL:       it.MediaURL,
		ConversationID: m.ConversationID,
		IsGroup:        it.IsGroup || m.IsGroup,
		GroupID:        orDefault(it.GroupID, m.GroupID),
		Timestamp:      ts,
		Extra: map[string]any{
			"account_id": m.AccountID, "bridge": true,
			"sender_type": orDefault(it.SenderType, m.SenderType),
		},
	}
	// 出站轮次 receiver_id 兜底：扩展侧 _historyItem 已填「会话对方」，此处再兜一层
	// （旧版扩展未填时，统一收信中心仍能按「对方」聚合而非「自己」）。
	if ev.ReceiverID == "" && it.Direction == "outbound" {
		ev.ReceiverID = m.ConversationID
	}
	if ev.IsGroup {
		ev.Extra["is_group"] = true
	}
	if ev.GroupID != "" {
		ev.Extra["group_id"] = ev.GroupID
	}
	if groupName := orDefault(it.GroupName, m.GroupName); groupName != "" {
		ev.Extra["group_name"] = groupName
	}
	return ev
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// toMessageEvent 将 UnifiedMessage 映射为 model.MessageEvent。
//
// HTTP-only 模式下：HTTP ingest 端点使用 httpMessageToEvent 转换 HTTP 上行消息；
// toMessageEvent 保留以兼容 (1) 老的统一收信中心读取路径 (2) 旧扩展帧格式 fallback。
//
// 会话级多轮 history 透传到 MessageEvent.History，并冗余到 Extra["history"]，
// 供统一收件箱展示/可观测（AI 上下文由 session_messages 自行重建）。
func toMessageEvent(m *UnifiedMessage) *model.MessageEvent {
	ch := ToBridgeChannel(m.Channel)
	ts := time.UnixMilli(m.Timestamp)
	if m.Timestamp == 0 {
		ts = time.Now()
	}
	ev := &model.MessageEvent{
		EventID:        m.EventID,
		SessionID:      ch + ":" + m.AccountID + ":" + m.ConversationID,
		Channel:        ch,
		SenderID:       m.SenderID,
		SenderName:     m.SenderName,
		SenderType:     m.SenderType,
		ReceiverID:     m.ReceiverID,
		MsgType:        m.MsgType,
		Content:        m.Content,
		MediaURL:       m.MediaURL,
		ConversationID: m.ConversationID,
		IsGroup:        m.IsGroup,
		GroupID:        m.GroupID,
		Timestamp:      ts,
		Extra:          map[string]any{"account_id": m.AccountID, "bridge": true, "sender_type": m.SenderType},
	}
	if len(m.History) > 0 {
		hist := make([]model.MessageEventHistoryItem, 0, len(m.History))
		for _, it := range m.History {
			if it == nil {
				continue
			}
			hist = append(hist, model.MessageEventHistoryItem{
				EventID:    it.EventID,
				SenderType: it.SenderType,
				SenderID:   it.SenderID,
				SenderName: it.SenderName,
				ReceiverID: it.ReceiverID,
				MsgType:    it.MsgType,
				Content:    it.Content,
				MediaURL:   it.MediaURL,
				Timestamp:  it.Timestamp,
				Direction:  it.Direction,
				IsGroup:    it.IsGroup,
				GroupID:    it.GroupID,
				GroupName:  it.GroupName,
			})
		}
		ev.History = hist
		ev.Extra["history"] = hist
	}
	if m.IsGroup {
		ev.Extra["is_group"] = true
	}
	if m.GroupID != "" {
		ev.Extra["group_id"] = m.GroupID
	}
	if m.GroupName != "" {
		ev.Extra["group_name"] = m.GroupName
	}
	return ev
}

// 编译期断言（防止 import 被优化掉）
var _ = json.Marshal
