package service

import (
	"context"

	"crypto/sha1"

	"crypto/sha256"

	"encoding/hex"

	"fmt"

	"os"

	"strconv"

	"strings"

	"sync"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hash/fnv"
	"hivemtk-user/internal/cache"
	"io"
)

type webhookJob struct {
	event  *model.WebhookEvent
	raw    []byte
	header map[string]string
	source string
	channel WebhookChannel
	account string
	payload *ParsedPayload
}

type tokenBucket struct {
	mu         sync.Mutex
	capacity   int
	refillRate float64
	tokens     float64
	lastRefill time.Time
	lastAccess time.Time 
}

const (
	WebhookDedupTTL = 5 * time.Minute

	WebhookWorkerCount = 4 

	WebhookQueueSize = 512

	WebhookRateLimit = 30

	WebhookRateBurst = 60

	WebhookMaxRetries = 3

	WebhookReplyConcurrency = 32
)

func webhookEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func groupNameFromHub(hub *model.MessageHub) string {
	if hub == nil || hub.Extra == nil {
		return ""
	}
	if v, ok := hub.Extra["group_name"]; ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	return ""
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

func sha1Hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

// ContentHashMsgID 基于「渠道 + 消息内容」生成稳定的消息ID（FNV-1a 32位 hex）。
// conversationID 不参与哈希 —— 同一文本在不同会话被 patrol 捕获时哈希一致，实现全局去重。
//
// 2026-08-05 根因修复（用户指定方案：消息ID用内容hash）：
//
//	核心问题：前端 sender_type 判定可能错误（把 AI 回复的 outbound 误判为 customer）。
//	正确方案：msg_id 只用稳定字段（channel + content），不含 sender_type/sender_id/conversationID/timestamp。
//	前端 contentHash() 用相同算法生成 event_id → 后端 outbound 的 msg_id 与之一致 → 前端 patrol 扫描 AI 回复时
//	生成的 event_id 与 DB msg_id 相同 → 钩子2 GetByMsgID 命中 → 跳过入库和 AI 触发，彻底解决回环。
//
// 2026-08-07 修正：去掉 conversationID。
//
//	同一 AI 回复可能被不同会话的 patrol 交叉捕获（DOM 切换残留），
//	若 contentHash 含 conversationID 则不同会话算不同消息 → GetByMsgID 漏检 → 回环未完全切断。
//
// 算法：FNV-1a 32位（与前端 types.js contentHash 完全一致，保证前后端结果相同）
//
//   - 输入：`channel|content`（content 去首尾空白）
//
//   - 输出：`mh:${hex}`（8位hex字符串，带 mh: 前缀便于日志识别）
//
//   - 锚点：ContentHashMsgID("douyin", "c1", "你好") == "mh:00550fed"（输入不含 conversationID；与前端 types.js::contentHash 逐字节一致）
//
//     ⚠️ 严禁在输入中加入 conversationID：message_hub.MsgID 采用 (msg_id, conversation_id) 复合唯一索引，
//     同一 AI 回复会被不同会话的 patrol 交叉捕获，含 conv 会让 GetByMsgID 漏检、回环去重失效（详见 commit 36509ab）。
func ContentHashMsgID(channel, conversationID, content string) string {

	s := channel + "|" + strings.TrimSpace(content)
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("mh:%08x", h.Sum32())
}

// ContentHashWithSender 统一收件去重哈希（渠道 + 发送者名称 + 消息内容）。
//
// 这是「渠道+发送者+消息内容」唯一去重依据的权威实现，前端（types.js::sharedContentHash）
// 与后端必须逐字节一致：
//   - 算法：FNV-1a 32 位，输入 channel|senderName|TrimSpace(content)，UTF-8 字节
//   - 输出：mh:<8位hex>
//
// 设计要点（与 ContentHashMsgID 的区别）：
//   - ContentHashMsgID 仅含渠道+内容，无法区分「AI 自己发的」与「客户复述了 AI 的原话」，
//     会导致客户复述被误判为回显而丢失（回环去重误杀）。
//   - 本函数把发送者纳入哈希，使「平台自己发出的消息」与「客户发的消息」拥有不同的去重键，
//     从而真正基于 (渠道,发送者,内容) 三元组做去重/自他判定。
//
// 发送者名称以服务端权威判定为准（见 inbox_ingress.senderKeyForDedup）：前端 patrol 上报的
// sender_type/sender_name 不可信（无法可靠分辨自他），服务端在入库前通过 DB 回查 message_hub
// 出站(outbound)行判定「自己消息」，再以账号(platform 身份)回填发送者，保证自/他区分不依赖前端标签。
//
// 注意：content 仍做 TrimSpace（与 ContentHashMsgID 保持一致，兼容首尾空白差异），
// 但严禁加入 conversationID——跨会话同内容(不同发送者)必须可区分，且复合唯一索引
// (msg_id, conversation_id) 已为跨会话同内容留出空间。
func ContentHashWithSender(channel, senderName, content string) string {
	s := channel + "|" + senderName + "|" + strings.TrimSpace(content)
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("mh:%08x", h.Sum32())
}

// isDuplicate 基于 eventID 的 TTL 幂等。
// 业务需要：外部渠道事件必须「恰好一次」处理。多实例下若各持进程内去重表，
// 重复投递会被不同实例各自放过 → 双处理。故改走全局缓存 SetNX：
//   - REDIS_HOST 配置时为 Redis 共享后端（跨实例去重）
//   - 否则为内存单例（单实例安全）
//
// TTL 内重复 key 已存在即命中返回 true；SetNX 异常时放行并告警（可用性优先）。
// 使用 context.Background() 而非入参 ctx：Bridge WS 生命周期短（重连循环会 cancel ctx），
// 而去重是基础设施功能，不应受连接生命周期影响。详见 trace ad589b80 双 orchestrator 根因。
func (s *WebhookService) isDuplicate(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}
	key := "mtk:webhook:dedup:" + eventID
	set, err := cache.GetGlobalCache().SetNX(context.Background(), key, "1", WebhookDedupTTL)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("event_id", eventID).Msg("[webhook] dedup 后端异常，放行")
		return false
	}
	if !set {

		logger.Ctx(ctx).Debug().Str("event_id", eventID).Msg("[webhook] dedup hit")
		return true
	}
	return false
}

func (s *WebhookService) allowRate(ctx context.Context, key string) bool {
	s.rlMu.Lock()
	b, ok := s.rlBuckets[key]
	if !ok {
		b = &tokenBucket{
			capacity:   WebhookRateBurst,
			refillRate: float64(WebhookRateLimit),
			tokens:     float64(WebhookRateBurst),
			lastRefill: time.Now(),
			lastAccess: time.Now(),
		}
		s.rlBuckets[key] = b
	}
	b.lastAccess = time.Now()
	s.rlMu.Unlock()
	return b.allow(context.Background())
}

// startRLJanitor 定期清理 idle 超过 5 分钟的限速桶（防内存泄漏）。
// tokenBucket 按 (channel, accountID) 为 key，若无清理则永久增长。
func (s *WebhookService) startRLJanitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.rlMu.Lock()
				cutoff := time.Now().Add(-5 * time.Minute)
				for k, b := range s.rlBuckets {
					if b.lastAccess.Before(cutoff) {
						delete(s.rlBuckets, k)
					}
				}
				s.rlMu.Unlock()
			}
		}
	}()
}

func (s *WebhookService) generateEventID(ctx context.Context, channel WebhookChannel, accountID string, body []byte) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + string(body)))
	return fmt.Sprintf("evt_%s", hex.EncodeToString(h[:8]))
}

func (s *WebhookService) genMessageID(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + p.Sender + ":" + p.Content + ":" + p.EventID))

	return fmt.Sprintf("msg_%s", hex.EncodeToString(h[:])[:22])
}

// TruncateForStore 截断防止 raw_data 过大
func (s *WebhookService) TruncateForStore(ctx context.Context, body []byte) string {
	const max = 64 * 1024
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...[truncated]"
}

func (s *WebhookService) getAccountSecret(ctx context.Context, platform, accountID string) (string, error) {
	if s.accountRepo == nil {
		return "", nil
	}

	if s.db == nil {
		return "", nil
	}
	// v3 审计 P1-6：优先按 (platform, accountID) 精确匹配；查不到再回退平台级单账号
	if acc, err := s.accountRepo.GetByPlatformAndAccount(ctx, platform, accountID); err == nil && acc != nil {
		return acc.APISecret, nil
	}
	acc, err := s.accountRepo.GetByPlatform(ctx, platform)
	if err != nil || acc == nil {
		return "", nil
	}
	return acc.APISecret, nil
}

// PendingCount 待处理事件数
func (s *WebhookService) PendingCount(ctx context.Context) int64 {
	if s.eventRepo == nil {
		return 0
	}
	c, _ := s.eventRepo.CountUnprocessed(ctx)
	return c
}

// QueueLen 队列长度
func (s *WebhookService) QueueLen(ctx context.Context) int { return len(s.queue) }

// ReadAll 读取请求体
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

// helpers
func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

