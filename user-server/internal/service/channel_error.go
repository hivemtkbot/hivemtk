package service

// 出站渠道错误分类与重试策略 —— ChatbotX「ChannelError{category, isRetryable}」模式移植（T4）。
//
// 设计：
//   - 各渠道 IntegrationService 现返回的错误多为 fmt.Errorf("... status %d: %s", code, body)，
//     无法拿到结构化状态码；本分类器从错误字符串提取状态码/平台错误码再分类，
//     避免为迁移而改动全部 IntegrationService 签名（surgical change）。
//   - 分类决定重试语义：限速（尊重 Retry-After / retry_after）、网络抖动（退避重试）、
//     鉴权失败（立即放弃 + 告警事件）、参数错误（立即放弃）、未知（按网络处理）。
//   - 新渠道接入时只需在其 IntegrationService 错误信息中保留 "status <code>" 惯例
//     （现状已满足），或直接返回 *ChannelError。

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ChannelErrorCategory 渠道错误类别
type ChannelErrorCategory string

const (
	CategoryRateLimited ChannelErrorCategory = "rate_limited"
	CategoryAuth        ChannelErrorCategory = "auth"
	CategoryNetwork     ChannelErrorCategory = "network"
	CategoryBadRequest  ChannelErrorCategory = "bad_request"
	CategoryUnknown     ChannelErrorCategory = "unknown"
)

// ChannelError 结构化渠道错误。实现 error 接口，可无损穿过现有 error 链路。
type ChannelError struct {
	Category   ChannelErrorCategory
	Retryable  bool
	StatusCode int           // HTTP 状态码（可得时）
	RetryAfter time.Duration // 限速时平台建议的等待（Retry-After / retry_after 秒）
	Raw        string        // 原始错误
}

func (e *ChannelError) Error() string { return "channel error [" + string(e.Category) + "]: " + e.Raw }

// AsChannelError 把任意 error 归一为 *ChannelError（已是则原样返回）。
func AsChannelError(err error) *ChannelError {
	if err == nil {
		return nil
	}
	var ce *ChannelError
	if errors.As(err, &ce) {
		return ce
	}
	return classifyChannelError(err.Error())
}

var (
	// \b 词边界防误匹配：如 "status 4001" 不得截断成 400（二次审查 S2 修复）
	reHTTPStatus = regexp.MustCompile(`status[ =:]+(\d{3})\b`)
	reRetryAfter = regexp.MustCompile(`retry_after[ ":]+(\d+)\b`)
)

// classifyChannelError 从错误字符串提取特征分类。
func classifyChannelError(raw string) *ChannelError {
	lower := strings.ToLower(raw)
	ce := &ChannelError{Raw: raw, Category: CategoryUnknown}

	if m := reHTTPStatus.FindStringSubmatch(lower); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n > 0 {
			ce.StatusCode = n
		}
	}

	switch {
	// 限速：HTTP 429 / WA 131048(配额)/131056(速率) / Telegram 429
	case ce.StatusCode == 429,
		strings.Contains(lower, "131048"),
		strings.Contains(lower, "131056"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "too many requests"):
		ce.Category = CategoryRateLimited
		ce.Retryable = true
		if m := reRetryAfter.FindStringSubmatch(raw); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				ce.RetryAfter = time.Duration(n) * time.Second
			}
		}

	// 鉴权/授权：401/403 / WA 131047 / token 类
	case ce.StatusCode == 401, ce.StatusCode == 403,
		strings.Contains(lower, "131047"),
		strings.Contains(lower, "invalid access token"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"):
		ce.Category = CategoryAuth
		ce.Retryable = false

	// 参数/业务不可重试：400/404/422
	case ce.StatusCode == 400, ce.StatusCode == 404, ce.StatusCode == 422:
		ce.Category = CategoryBadRequest
		ce.Retryable = false

	// 网络类：5xx 或连接类关键字（net.Error 类型无法从字符串还原，走关键字）
	case ce.StatusCode >= 500,
		strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "no such host"):
		ce.Category = CategoryNetwork
		ce.Retryable = true

	default:
		// 未知错误保守处理：可重试（与现有 retryWithBackoff 语义一致）
		ce.Category = CategoryUnknown
		ce.Retryable = true
	}
	return ce
}

// retryDelaysFor 按分类产出重试延迟序列。
// RateLimited：平台给了 RetryAfter 就只等它（单次）；否则走默认指数退避。
// Network/Unknown：沿用仓库既有 2s/10s/30s。
// 不可重试分类返回 nil（调用方立即放弃）。
func retryDelaysFor(ce *ChannelError) []time.Duration {
	if ce == nil {
		return []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	}
	switch ce.Category {
	case CategoryRateLimited:
		if ce.RetryAfter > 0 {
			return []time.Duration{ce.RetryAfter}
		}
		return []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	case CategoryNetwork, CategoryUnknown:
		return []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	default: // auth / bad_request
		return nil
	}
}

// outboundSendFailed 官方渠道出站失败的统一收敛点（T4）。
//
// 行为：
//  1. 分类错误并以类别+状态码输出结构化日志（替代裸 Errorf）；
//  2. 不可重试（auth/bad_request）→ message_hub 出站行标记 send_failed，
//     Extra 记录分类与原因——管理端可见、可对账，不再"静默丢失"；
//  3. 鉴权失败额外发 operation.log 事件（现有总线），触发运维告警链路；
//  4. 可重试错误（限速/网络）此处不自动补发——出站补发有内容重复风险，
//     交给上层编排的既有重试语义（retryWithBackoff 按分类调参）。
//
// best-effort：hubMsg 为 nil（AI 路径现场构造前的失败）或行更新失败仅记日志。
func (s *WebhookService) outboundSendFailed(ctx context.Context, channel WebhookChannel, accountID string, hubMsg *model.MessageHub, sendErr error) {
	ce := AsChannelError(sendErr)
	logger.Ctx(ctx).Error().
		Str("channel", string(channel)).
		Str("account_id", accountID).
		Str("category", string(ce.Category)).
		Int("status_code", ce.StatusCode).
		Bool("retryable", ce.Retryable).
		Msg("outbound send failed: " + ce.Raw)

	// 鉴权失败告警：token 失效需要人工介入，靠重试无法自愈
	if ce.Category == CategoryAuth {
		if bus := event.GetGlobalBus(); bus != nil {
			bus.Publish(event.Event{
				Topic:     event.TopicOperationLog,
				Source:    "webhook_outbound",
				Timestamp: time.Now(),
				Payload: event.OperationLogPayload{
					Action:     "outbound_auth_failure",
					Module:     "channel",
					Resource:   string(channel),
					ResourceID: accountID,
				},
			})
		}
	}

	if hubMsg == nil || s.messageHubRepo == nil {
		return
	}
	// 可重试错误不落终态，避免误导管理端；不可重试才标记 send_failed
	if ce.Retryable {
		return
	}
	extra := model.JSONMap{}
	for k, v := range hubMsg.Extra {
		extra[k] = v
	}
	extra["send_failed_category"] = string(ce.Category)
	extra["send_failed_reason"] = ce.Raw
	// 二次审查 S1 修复：必须走列级 Updates（Save 全列覆盖会清空 msg_id/content 等字段）
	_ = s.messageHubRepo.MarkOutboundSendFailed(ctx, hubMsg.ID, extra)
}
