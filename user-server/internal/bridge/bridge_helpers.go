package bridge

import (
	"context"
	"net/url"

	gw "hivemtk-user/internal/channelgw"
	"hivemtk-user/internal/model"
)


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
// 2026-08-10 协议单源化：委托 channelgw.HistoryToEvent（HTTP/WS 传输共用同一转换器）。
func historyItemToEvent(m *UnifiedMessage, it *HistoryItem) *model.MessageEvent {
	return gw.HistoryToEvent(m, it)
}

// toMessageEvent 将 UnifiedMessage 映射为 model.MessageEvent。
//
// 2026-08-10 协议单源化：委托 channelgw 规范化转换器 ToEventFull（含 History 拷贝），
// 保留本函数以兼容 (1) 老的统一收信中心读取路径 (2) 旧扩展帧格式 fallback。
func toMessageEvent(m *UnifiedMessage) *model.MessageEvent {
	if m == nil {
		return nil
	}
	m.Channel = ToBridgeChannel(m.Channel)
	return m.ToEventFull("http")
}

