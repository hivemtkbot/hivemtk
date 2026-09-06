package service

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
	StatusCode int
	RetryAfter time.Duration
	Raw        string
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
	reHTTPStatus = regexp.MustCompile(`status[ =:]+(\d{3})\b`)
	reRetryAfter = regexp.MustCompile(`retry_after[ ":]+(\d+)\b`)
)

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

	case ce.StatusCode == 401, ce.StatusCode == 403,
		strings.Contains(lower, "131047"),
		strings.Contains(lower, "invalid access token"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"):
		ce.Category = CategoryAuth
		ce.Retryable = false

	case ce.StatusCode == 400, ce.StatusCode == 404, ce.StatusCode == 422:
		ce.Category = CategoryBadRequest
		ce.Retryable = false

	case ce.StatusCode >= 500,
		strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "no such host"):
		ce.Category = CategoryNetwork
		ce.Retryable = true

	default:

		ce.Category = CategoryUnknown
		ce.Retryable = true
	}
	return ce
}

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
	default:
		return nil
	}
}

func (s *WebhookService) outboundSendFailed(ctx context.Context, channel WebhookChannel, accountID string, hubMsg *model.MessageHub, sendErr error) {
	ce := AsChannelError(sendErr)
	logger.Ctx(ctx).Error().
		Str("channel", string(channel)).
		Str("account_id", accountID).
		Str("category", string(ce.Category)).
		Int("status_code", ce.StatusCode).
		Bool("retryable", ce.Retryable).
		Msg("outbound send failed: " + ce.Raw)

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

	if ce.Retryable {
		return
	}
	extra := model.JSONMap{}
	for k, v := range hubMsg.Extra {
		extra[k] = v
	}
	extra["send_failed_category"] = string(ce.Category)
	extra["send_failed_reason"] = ce.Raw

	_ = s.messageHubRepo.MarkOutboundSendFailed(ctx, hubMsg.ID, extra)
}
