package service

// 会话级防抖聚合推理 —— ChatbotX「dedup 闸门 + Redis list 收集器 + 延迟消费取全量」模式移植（T5）。
//
// 背景：HandleIngressMessage 单条路径逐条触发 AI。用户 10 秒内连发 3 条碎片消息
// （"在吗" / "有个问题" / "就是那个订单"）会触发 3 次 AI 推理，前两次拿到孤立
// 上下文生成无效回复。本模块在触发前做会话级防抖：
//
//	第 1 条：AppendPendingMessage 收集 + SetNX 抢门闸 → 赢家调度 AfterFunc(窗口)
//	第 2..N 条：门闸已被占 → 仅收集进 Redis list，不触发
//	窗口到期：赢家 PopPendingMessages 取全量 → 按时间序合并为单条内容 → 触发 1 次 AI
//	门闸释放 → 新消息可开下一窗口
//
// ChatbotX 语义对齐：
//   - 旧触发不取消而是复用（赢家即调度者）；
//   - 聚合发生在消费时刻而非入队时刻（延迟任务只带首条，内容取自收集器全量）；
//   - 转人工关键词豁免（对齐 ChatbotX "关键词命中不延迟"）；
//   - 非文本消息（媒体）不聚合，立即触发（AI 需要即时处理图片等）。
//
// 已知取舍（记录于设计文档）：
//   - AfterFunc 为进程内存活——进程崩溃则本窗口聚合丢失，pending list 5min TTL
//     自然回收，效果等同该消息未触发 AI（与桥接渠道 best-effort 语义一致）；
//     恢复能力由 webhook 恢复扫描器（T1）覆盖官方渠道事件级重放。
//   - 窗口默认禁用（渐进上线）：生产部署显式设置 AI_DEBOUNCE_SECONDS=3 开启，
//     0 = 禁用（退回逐条触发）；事件 Extra["ai_debounce_seconds"] 可按账号
//     覆盖（预留 per-account 接线）。
import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

const (
	aiDebounceGatePrefix  = "hivemtk:ai:debounce:"
	aiDebounceMaxSeconds  = 30
	aiDebounceMaxMessages = 20 // 单窗口聚合上限，防极端刷屏撑爆 prompt
)

// aiDebounceSeconds 计算本事件的防抖窗口秒数（0 = 禁用）。
// 优先级：事件 Extra["ai_debounce_seconds"]（账号级，预留）> 环境变量 > 默认 0。
//
// 默认禁用的理由：防抖引入 N 秒回复延迟是产品行为变更，渐进上线——
// 生产部署显式设置 AI_DEBOUNCE_SECONDS=3 开启；单测保持确定性（触发即到）。
func aiDebounceSeconds(event *model.MessageEvent) int {
	def := 0
	if v := os.Getenv("AI_DEBOUNCE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			def = n
		}
	}
	if event != nil && event.Extra != nil {
		if v, ok := event.Extra["ai_debounce_seconds"]; ok {
			switch t := v.(type) {
			case int:
				if t >= 0 {
					return min(t, aiDebounceMaxSeconds)
				}
			case int64:
				if t >= 0 {
					return min(int(t), aiDebounceMaxSeconds)
				}
			case float64:
				if t >= 0 {
					return min(int(t), aiDebounceMaxSeconds)
				}
			}
		}
	}
	if def < 0 {
		return 0
	}
	return min(def, aiDebounceMaxSeconds)
}

// triggerAIWithDebounce 带防抖的 AI 触发（仅 HandleIngressMessage 单条路径使用；
// Recheck 补触发与 Batch 合并路径直接调 triggerAIForEvent，不经过本函数）。
func (s *InboxIngressService) triggerAIWithDebounce(ctx context.Context, event *model.MessageEvent) {
	seconds := aiDebounceSeconds(event)
	content := strings.TrimSpace(event.Content)
	// 禁用 / 无缓存后端 / 非文本内容 / 转人工关键词命中 → 立即触发（豁免）
	if seconds <= 0 || s.cache == nil || content == "" ||
		MatchTransferKeywords(content) || MatchExplicitKeywords(content) {
		s.triggerAIForEvent(ctx, event)
		return
	}
	sessionID := event.SessionID
	if sessionID == "" {
		s.triggerAIForEvent(ctx, event)
		return
	}

	// 收集本条进窗口（Redis list，TTL 兜底防孤儿键）
	if err := s.AppendPendingMessage(ctx, sessionID, content); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("session_id", sessionID).Msg("[Inbox][Debounce] append pending failed, fallback to immediate trigger")
		s.triggerAIForEvent(ctx, event)
		return
	}

	// ChatbotX 语义：窗口内只有赢家负责调度合并触发；其余消息静默收集。
	// 门闸值 = 本赢家 token：窗口到期回调 ReleaseLock 校验持有者，防止回调延迟
	// 越过 TTL 后盲删下一个窗口的门闸（二次审查 S2 修复）。
	gate := aiDebounceGatePrefix + sessionID
	gateToken := uuid.NewString()
	ok, err := s.cache.SetNX(ctx, gate, gateToken, time.Duration(seconds)*time.Second+time.Second)
	if err != nil {
		// 门闸后端异常：退化为立即触发（fail-open，与 interceptInbound 同哲学）
		logger.Ctx(ctx).Warn().Err(err).Str("session_id", sessionID).Msg("[Inbox][Debounce] gate backend error, fallback to immediate trigger")
		// 本条已入 pending list：回滚取出，避免下一窗口重复喂 AI（二次审查 S4 修复）
		s.drainOnePending(ctx, sessionID)
		s.triggerAIForEvent(ctx, event)
		return
	}
	if !ok {
		logger.Ctx(ctx).Info().
			Str("session_id", sessionID).
			Str("conv_id", event.ConversationID).
			Int("debounce_seconds", seconds).
			Msg("[Inbox][Debounce] window active, message coalesced (waiting for window close)")
		return
	}

	// 赢家：窗口到期后合并全量 pending 消息，单次触发
	latestContent := content
	mergedEvent := *event
	// 二次审查 S2 修复：深拷贝 Extra——浅拷贝与原 event 共享 map，
	// 回调 goroutine 写 ai_debounce_merged 会与持有原 event 的读方竞态
	if event.Extra != nil {
		extraCopy := make(map[string]any, len(event.Extra)+1)
		for k, v := range event.Extra {
			extraCopy[k] = v
		}
		mergedEvent.Extra = extraCopy
	}
	time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		// 请求级 ctx 已随 HTTP 返回取消，窗口回调用独立 context
		bctx := context.Background()
		pending, perr := s.PopPendingMessages(bctx, sessionID)
		// 仅当仍是本赢家持有时才释放门闸（ReleaseLock 校验 token）：
		// 回调延迟越过 TTL、新窗口已开时不会误删新门闸
		_, _ = s.cache.ReleaseLock(bctx, gate, gateToken)
		if perr != nil {
			logger.Ctx(bctx).Warn().Err(perr).Str("session_id", sessionID).Msg("[Inbox][Debounce] pop pending failed, trigger with latest only")
			pending = []string{latestContent}
		}
		if len(pending) == 0 {
			pending = []string{latestContent}
		}
		// LPush 逆序（最新在前）→ coalescePending 截断超限后反转为时间序
		pending = coalescePending(pending)

		mergedEvent.Content = strings.Join(pending, "\n")
		if mergedEvent.Extra == nil {
			mergedEvent.Extra = map[string]any{}
		}
		mergedEvent.Extra["ai_debounce_merged"] = len(pending)

		logger.Ctx(bctx).Info().
			Str("session_id", sessionID).
			Str("conv_id", mergedEvent.ConversationID).
			Int("merged_count", len(pending)).
			Int("content_len", len(mergedEvent.Content)).
			Msg("[Inbox][Debounce] window closed, triggering AI with merged messages")

		s.triggerAIForEvent(bctx, &mergedEvent)
	})
}

// drainOnePending 从 pending list 弹出一条（fail-open 回滚用）。
// LPop 一条即本次 append 的那条（list 头部为最新写入）。
func (s *InboxIngressService) drainOnePending(ctx context.Context, sessionID string) {
	if s.cache == nil || sessionID == "" {
		return
	}
	if _, err := s.cache.LPop(ctx, InboxPendingKey+sessionID); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("session_id", sessionID).Msg("[Inbox][Debounce] drain pending failed (TTL will reclaim)")
	}
}

// mergePending 注入桩测试辅助：暴露窗口到期的合并逻辑（时间序 + 超限截断）。
func coalescePending(pending []string) []string {
	if len(pending) > aiDebounceMaxMessages {
		pending = pending[:aiDebounceMaxMessages]
	}
	for i, j := 0, len(pending)-1; i < j; i, j = i+1, j-1 {
		pending[i], pending[j] = pending[j], pending[i]
	}
	return pending
}
