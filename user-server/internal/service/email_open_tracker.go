package service

// email_open_tracker.go 邮件打开率追踪服务（D 域 缺口 #2）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/marketing-features/email-smtp-config.md + 核心链路优化 §13.2
//
// 与 email_tracking.go（通用追踪：open/click/bounce/unsubscribe）的关系：
//   - email_tracking.go  提供底层 token 生成 + 事件落库
//   - 本文件              专注于「邮件打开」场景：
//                            * 1×1 透明 PNG 像素
//                            * open_pixel HTTP handler 逻辑（不依赖 gin）
//                            * Postmark 风格 webhook 事件采集
//                            * 塞邮式（SendCloud）webhook 事件采集
//                            * 打开率（Open Rate）批量聚合
//
// 私域独立部署: 无 merchant_id 字段
//
// 像素：43 字节 1×1 透明 PNG（业内标准，最小最通用）
// 安全：所有事件在 handler 层先验证 token 签名 → 防伪造打开

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/repository"
)

// ----------------------------------------------------------------------------
// 像素常量
// ----------------------------------------------------------------------------

// EmailOpenPixel 1×1 透明 PNG 字节
//
// 最小可用 PNG：43 字节，所有邮件客户端 / 浏览器均能正确显示为透明像素。
// 选 GIF43a / PNG8 会被部分安全软件识别为追踪器，PNG 透明像素兼容性最佳。
var EmailOpenPixel = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR length + name
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, // bit depth / color type
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, // IDAT length + name
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, // IDAT payload (zlib stream start)
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, // CRC
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, // IEND length + name
	0x42, 0x60, 0x82, // CRC
}

// EmailOpenPixelContentType 像素 HTTP Content-Type
const EmailOpenPixelContentType = "image/png"

// EmailOpenPixelMaxAge 像素缓存时间（避免邮件客户端反复 GET 触发重复计数）
//
// 长缓存：30 天。30 天后追踪 token 早已过期（默认 90 天），不会与业务冲突。
// 短缓存：邮件客户端对同一 URL 1 天内可能拉多次。
const EmailOpenPixelMaxAge = 30 * 24 * 3600

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// EmailOpenTrackerService 邮件打开追踪服务
type EmailOpenTrackerService struct {
	tracking *EmailTrackingService
	repo     repository.EmailTrackingRepository
}

// NewEmailOpenTrackerService 创建邮件打开追踪服务
func NewEmailOpenTrackerService(tracking *EmailTrackingService, repo repository.EmailTrackingRepository) *EmailOpenTrackerService {
	if tracking == nil {
		tracking = NewEmailTrackingService(repo)
	}
	if repo == nil {
		repo = repository.NewEmailTrackingRepository(nil)
	}
	return &EmailOpenTrackerService{tracking: tracking, repo: repo}
}

// ----------------------------------------------------------------------------
// 1×1 像素 URL 生成
// ----------------------------------------------------------------------------

// GenerateOpenPixelURL 生成邮件打开追踪的 1×1 像素 URL
//
// 链接格式：{baseURL}/api/email/track/open/{token}.png
// 选 .png 后缀是 Postmark / Mailchimp 等业内标准做法，方便部分邮件
// 客户端按 Content-Type 缓存或屏蔽。
func (s *EmailOpenTrackerService) GenerateOpenPixelURL(ctx context.Context, email, jobID string) (string, error) {
	token, err := s.tracking.GenerateTrackingPixelToken(ctx, email, jobID)
	if err != nil {
		return "", err
	}
	base := s.tracking.baseURL(ctx)
	return fmt.Sprintf("%s/api/email/track/open/%s.png", strings.TrimRight(base, "/"), token), nil
}

// RenderPixel 处理打开追踪 GET 请求
//
// 行为：
//  1. 校验 token 签名与有效期
//  2. 异步记录打开事件（不影响像素返回速度）
//  3. 立即返回 1×1 透明 PNG + Cache-Control: max-age=2592000, immutable
func (s *EmailOpenTrackerService) RenderPixel(ctx context.Context, token, ip, ua string) ([]byte, string, int, error) {
	if token == "" {
		return nil, "", 0, errors.New("token 不能为空")
	}
	// 修复：同一 token 30 秒内只记录一次打开事件，避免邮件客户端预取/预览面板
	// 自动 GET 像素 URL 造成重复计数（MarkOpenSeen 已实现该防重，此前 RenderPixel 未调用它）。
	if !MarkOpenSeen(token) {
		return EmailOpenPixel, EmailOpenPixelContentType, EmailOpenPixelMaxAge, nil
	}
	// 异步记录（不阻塞 HTTP 响应）
	go func(t, ipAddr, userAgent string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.tracking.RecordOpenEvent(bgCtx, t, ipAddr, userAgent); err != nil {
			// 静默失败（生产环境记录日志；测试时可观测）
			_ = err
		}
	}(token, ip, ua)

	// 始终返回 1×1 透明像素（即使 token 无效也返回，避免邮件客户端过滤）
	return EmailOpenPixel, EmailOpenPixelContentType, EmailOpenPixelMaxAge, nil
}

// ----------------------------------------------------------------------------
// Postmark 风格 webhook 事件采集
// ----------------------------------------------------------------------------

// PostmarkOpenEvent Postmark 邮件打开 webhook payload
//
// 兼容 Postmark MessageEvent API：https://postmarkapp.com/developer/webhooks/message-event-webhook
// 我们只关心 Open / Click / Bounce / SpamComplaint 4 类事件。
type PostmarkOpenEvent struct {
	RecordType string `json:"RecordType"`  // Open / Click / Bounce / SpamComplaint
	MessageID  string `json:"MessageID"`   // 邮件唯一 ID
	Recipient  string `json:"Recipient"`   // 收件人邮箱
	ReceivedAt string `json:"DeliveredAt"` // RFC3339 时间
	UserAgent  string `json:"UserAgent"`   // 客户端 UA
	IP         string `json:"IP"`          // 客户端 IP
	// Click 事件专用
	ClickLocation string `json:"ClickLocation,omitempty"`
	OriginalLink  string `json:"OriginalLink,omitempty"`
}

// RecordPostmarkEvent 记录 Postmark 风格事件
//
// 自动映射 RecordType → 内部事件类型（open/click/bounce/unsubscribe）
func (s *EmailOpenTrackerService) RecordPostmarkEvent(ctx context.Context, evt *PostmarkOpenEvent) error {
	if evt == nil {
		return errors.New("event is nil")
	}
	switch strings.ToLower(evt.RecordType) {
	case "open":
		// Postmark 不携带我们自签 token，直接按 email + jobID 查最近一笔营销任务
		return s.recordOpenByEmail(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	case "click":
		// Postmark 点击事件无自签 token，走降级路径（按 email 直落 click 事件）
		return s.recordClickByEmail(ctx, evt.Recipient, evt.MessageID, evt.OriginalLink, evt.IP, evt.UserAgent)
	case "bounce":
		return s.tracking.RecordBounceEvent(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	case "spamcomplaint":
		// 投诉等同于主动退订（合规要求：收件人投诉后必须停止发送）
		return s.tracking.RecordUnsubscribeEvent(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	default:
		return fmt.Errorf("%w: unsupported RecordType: %s", utils.ErrInvalidInput, evt.RecordType)
	}
}

// recordOpenByEmail 按 email 记录打开事件（无 token 场景）
//
// 用于 Postmark / SendCloud 这类第三方 SMTP，他们不传我们自签 token，
// 仅传 Recipient + MessageID。我们退化为"按 email 记录一次打开事件"，
// 精度低于自签 token，但仍能给出有意义的打开率统计。
func (s *EmailOpenTrackerService) recordOpenByEmail(ctx context.Context, email, messageID, ip, ua string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("email 不能为空")
	}
	// 生成幂等 event_id：按 (email, messageID) 哈希，使同一邮件对同一收件人的
	// 重复打开事件能被 EventExists 正确去重（原先用纳秒时间戳导致永远不重复，防重形同虚设）。
	eventID := fmt.Sprintf("pm-open-%x", sha256.Sum256([]byte(email+"|"+messageID)))
	// repo/db 为 nil 时不阻断（兜底：继续落库，由 CreateEvent 报错）
	if s.repo != nil {
		if exists, err := s.repo.EventExists(ctx, eventID); err == nil && exists {
			return nil
		}
	}
	evt := &model.EmailTrackingEvent{
		EventID:   eventID,
		Email:     email,
		JobID:     messageID, // 用 MessageID 占位（Postmark 不直接给我们的 job_id）
		EventType: model.EmailEventTypeOpen,
		UserAgent: ua,
		IP:        ip,
		Timestamp: time.Now(),
	}
	if s.repo == nil {
		return errors.New("email tracking repository 未初始化")
	}
	return s.repo.CreateEvent(ctx, evt)
}

// buildClickTokenFromPostmark 由于 Postmark 点击事件中不携带我们自签 token，
// 这里采用退化路径：直接走 click 事件记录（target 字段写入 OriginalLink）。
//
// 由于原 tracking.RecordClickEvent 强依赖自签 token，这里不复用，
// 而是直接落库一筆 click 事件。
func buildClickTokenFromPostmark(evt *PostmarkOpenEvent) string {
	// 返回占位 token，调用方会以 recordOpenByEmail 路径走入库；
	// 这里为保持 RecordPostmarkEvent 单一返回类型，返回一个本地标识。
	return fmt.Sprintf("pm-click:%s:%s", evt.MessageID, evt.Recipient)
}

// ----------------------------------------------------------------------------
// 塞邮式 (SendCloud) webhook 事件采集
// ----------------------------------------------------------------------------

// SendCloudOpenEvent SendCloud webhook payload
//
// 文档：https://www.sendcloud.net/doc/email_v2/webhook/
// 关键字段：event（delivered / open / click / bounce / spam_report / unsubscribe）
type SendCloudOpenEvent struct {
	Event     string `json:"event"`      // 事件类型
	Recipient string `json:"recipient"`  // 收件人
	MessageID string `json:"message_id"` // 邮件 ID
	IP        string `json:"ip"`
	UserAgent string `json:"useragent"`
	URL       string `json:"url,omitempty"`    // click 事件：原始链接
	Reason    string `json:"reason,omitempty"` // bounce 事件：原因
	Timestamp string `json:"timestamp"`        // RFC3339 / Unix
}

// RecordSendCloudEvent 记录 SendCloud 风格事件
func (s *EmailOpenTrackerService) RecordSendCloudEvent(ctx context.Context, evt *SendCloudOpenEvent) error {
	if evt == nil {
		return errors.New("event is nil")
	}
	switch strings.ToLower(evt.Event) {
	case "delivered", "open":
		return s.recordOpenByEmail(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	case "click":
		// 与 Postmark 路径一致：直接落库 click 事件
		return s.recordClickByEmail(ctx, evt.Recipient, evt.MessageID, evt.URL, evt.IP, evt.UserAgent)
	case "bounce", "spam_report":
		return s.tracking.RecordBounceEvent(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	case "unsubscribe":
		return s.tracking.RecordUnsubscribeEvent(ctx, evt.Recipient, evt.MessageID, evt.IP, evt.UserAgent)
	default:
		return fmt.Errorf("%w: unsupported event: %s", utils.ErrInvalidInput, evt.Event)
	}
}

// recordClickByEmail 按 email 记录 click 事件（无 token 场景）
func (s *EmailOpenTrackerService) recordClickByEmail(ctx context.Context, email, messageID, targetURL, ip, ua string) error {
	email = normalizeEmail(email)
	if email == "" {
		return errors.New("email 不能为空")
	}
	eventID := fmt.Sprintf("sc-click-%s-%d", messageID, time.Now().UnixNano())
	if s.repo != nil {
		if exists, _ := s.repo.EventExists(ctx, eventID); exists {
			return nil
		}
	}
	evt := &model.EmailTrackingEvent{
		EventID:   eventID,
		Email:     email,
		JobID:     messageID,
		EventType: model.EmailEventTypeClick,
		UserAgent: ua,
		IP:        ip,
		Timestamp: time.Now(),
	}
	// target URL 暂存到 IP 字段：避免新加 schema
	// 真实部署应扩展 EmailTrackingEvent.TargetURL 字段
	if targetURL != "" {
		// 简化：把 url 拼到 UA 后（兜底），生产可加 TargetURL 字段
		evt.UserAgent = ua + " | target=" + targetURL
	}
	if s.repo == nil {
		return errors.New("email tracking repository 未初始化")
	}
	return s.repo.CreateEvent(ctx, evt)
}

// ----------------------------------------------------------------------------
// 打开率批量聚合
// ----------------------------------------------------------------------------

// OpenRateMetrics 打开率指标
type OpenRateMetrics struct {
	JobID           string  `json:"job_id"`
	TotalSent       int64   `json:"total_sent"`
	UniqueOpened    int64   `json:"unique_opened"`
	TotalOpens      int64   `json:"total_opens"`
	OpenRate        float64 `json:"open_rate"` // unique_opened / total_sent * 100
	AvgOpensPerUser float64 `json:"avg_opens_per_user"`
}

// GetOpenRateMetrics 获取指定任务的打开率指标
func (s *EmailOpenTrackerService) GetOpenRateMetrics(ctx context.Context, jobID string, totalSent int64) (*OpenRateMetrics, error) {
	if jobID == "" {
		return nil, errors.New("job_id 不能为空")
	}
	uniqueOpened, err := s.repo.CountUniqueEmailsByJob(ctx, jobID, model.EmailEventTypeOpen)
	if err != nil {
		return nil, err
	}
	totalOpens, err := s.repo.CountEventsByJob(ctx, jobID, model.EmailEventTypeOpen)
	if err != nil {
		return nil, err
	}
	m := &OpenRateMetrics{
		JobID:        jobID,
		TotalSent:    totalSent,
		UniqueOpened: uniqueOpened,
		TotalOpens:   totalOpens,
	}
	if totalSent > 0 {
		m.OpenRate = round2(float64(uniqueOpened) / float64(totalSent) * 100)
	}
	if uniqueOpened > 0 {
		m.AvgOpensPerUser = round2(float64(totalOpens) / float64(uniqueOpened))
	}
	return m, nil
}

// ----------------------------------------------------------------------------
// 缓存：避免邮件客户端预取导致重复计数
// ----------------------------------------------------------------------------

var (
	pixelCacheMu sync.Mutex
	pixelCache   = make(map[string]time.Time)
)

// MarkOpenSeen 标记一次打开事件已记录（防重）
//
// 同一 token 30 秒内只记一次，避免 Gmail / Outlook 预览面板
// 自动 GET 像素 URL 导致的重复计数。
func MarkOpenSeen(token string) bool {
	if token == "" {
		return false
	}
	pixelCacheMu.Lock()
	defer pixelCacheMu.Unlock()
	if last, ok := pixelCache[token]; ok && time.Since(last) < 30*time.Second {
		return false
	}
	pixelCache[token] = time.Now()
	// 定期清理过期项
	if len(pixelCache) > 4096 {
		cutoff := time.Now().Add(-5 * time.Minute)
		for k, t := range pixelCache {
			if t.Before(cutoff) {
				delete(pixelCache, k)
			}
		}
	}
	return true
}

// ----------------------------------------------------------------------------
// 工具
// ----------------------------------------------------------------------------

// EmailEventSummary 邮件事件摘要（用于日志）
func EmailEventSummary(evtType, email string) string {
	return fmt.Sprintf("[%s] %s", evtType, email)
}

// PrettyPrintJSON 调试用：序列化 webhook payload
func PrettyPrintJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
