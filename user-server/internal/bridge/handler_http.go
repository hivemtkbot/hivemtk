package bridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/channelgw"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/metrics"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)


// HTTP 端点参数（与 WS 端点默认值严格对齐，单源来自 DEFAULTS.md）
const (
	HTTPPollingMaxTimeout = 500 * time.Second
	HTTPPollingDefaultTimeout = 30 * time.Second
	HTTPIngestMaxBodySize = 4 << 20 
	HTTPIngestMaxMessages = 200
)

// HTTPIngestRequest 上报请求体（别名 channelgw.IngestRequest，与扩展端 http-ingest.js 严格对齐）。
// 2026-08-10 协议单源化：线路协议类型统一收敛到渠道网关 channelgw，HTTP/WS 共用。
type HTTPIngestRequest = channelgw.IngestRequest

// HTTPIngestMessage 单条消息（别名 channelgw.IngestMessage，与前端 types.js UnifiedMessage 对齐）。
//
// SenderType 仅供前端参考；服务端在入库环节按“内容是否命中本会话平台下发(outbound)”
// 权威重判自/他（见 InboxIngressService.isPlatformOutboundEcho），不再信任此字段。
// 故：命中 outbound → 视为平台自己的回显(SELF)跳过；否则一律按用户消息(CUSTOMER)处理。
type HTTPIngestMessage = channelgw.IngestMessage

// HTTPIngestResponse 上报响应（别名 channelgw.IngestResponse）。
type HTTPIngestResponse = channelgw.IngestResponse

// HTTPIngestResult 单条消息处理结果（别名 channelgw.IngestResult）。
type HTTPIngestResult = channelgw.IngestResult

// HTTPIngestPending 等待中的长轮询请求（用于将来"扩展发来多轮等 AI 一起返回"扩展）
//
// 当前实现：单条消息的 AI 回复即时在请求内完成；本结构为预留，便于将来"批量合并推理"
// （同会话多条消息 → 合并为单次 AI 调用 → 统一返回）。当前不写、不读，保留接口稳定。
type HTTPIngestPending struct {
}

// collectHTTPRequestInfo 收集 HTTP 请求的"完整 URL + 全部 query + headers + body"快照。
// 2026-08-05 重构：与 WS 端 collectBridgeWSRequestInfo 同源设计，
// 5 个日志点（收到请求 / 参数缺失 / 渠道拒绝 / ingest 处理 / 响应回写）共用同一结构，
// 便于日志检索串联（按 full_url 或 channel+account_id 聚合时一次抓全）。
type httpRequestInfo struct {
	Method         string
	Path           string
	RawQuery       string
	FullURL        string
	ContentType    string
	ContentLength  int64
	Channel        string
	AccountID      string
	ConversationID string
	TokenMasked    string
	RemoteAddr     string
	Origin         string
	UserAgent      string
	ParsedQuery    map[string]string
	BodyPreview    string 
	BodySize       int
}

// collectHTTPRequestInfo 从 gin.Context 提取 HTTP ingest 请求快照。
// 不修改 c、不做业务校验，纯粹是字段收集 + token 脱敏。
//
// 行为说明：
//   - 读取完整 body 用于下游 BindJSON；同时截取前 4KB 作为日志预览
//   - body 读取后必须写回 c.Request.Body，否则下游 BindJSON 读不到
//   - 解析失败不返回错误（让 gin BindJSON 报更明确的错）
func collectHTTPRequestInfo(c *gin.Context) httpRequestInfo {
	token := c.Query("token")
	origContentLength := c.Request.ContentLength
	fullBody, bodySize, bodyPreview, _ := readBodyForLog(c.Request.Body, 4096)
	if c.Request != nil && c.Request.Body != nil {
		c.Request.Body = io.NopCloser(strings.NewReader(fullBody))
		c.Request.ContentLength = int64(len(fullBody))
	}
	return httpRequestInfo{
		Method:         c.Request.Method,
		Path:           c.Request.URL.Path,
		RawQuery:       c.Request.URL.RawQuery,
		FullURL:        c.Request.URL.String(),
		ContentType:    c.GetHeader("Content-Type"),
		ContentLength:  origContentLength,
		Channel:        c.Query("channel"),
		AccountID:      c.Query("account_id"),
		ConversationID: c.Query("conversation_id"),
		TokenMasked:    maskTokenBridge(token),
		RemoteAddr:     c.Request.RemoteAddr,
		Origin:         c.GetHeader("Origin"),
		UserAgent:      c.GetHeader("User-Agent"),
		ParsedQuery:    describeUpstreamQuery(c.Request.URL.Query()),
		BodyPreview:    bodyPreview,
		BodySize:       bodySize,
	}
}

// readBodyForLog 读取 body 全部内容。
// 返回 (fullBody, totalSize, preview, truncated)：
//   - fullBody: 完整 body 字符串（写回 c.Request.Body 供下游 BindJSON 使用）
//   - totalSize: 完整 body 字节数
//   - preview: 前 N 字节预览（用于日志，超出截断并追加提示）
//   - truncated: body 是否超过 previewBytes
func readBodyForLog(rc io.ReadCloser, previewBytes int) (string, int, string, bool) {
	if rc == nil {
		return "", 0, "", false
	}
	all, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		msg := fmt.Sprintf("[read body failed: %v]", err)
		return msg, 0, msg, false
	}
	total := len(all)
	full := string(all)
	if total <= previewBytes {
		return full, total, full, false
	}
	preview := string(all[:previewBytes]) + fmt.Sprintf("... [truncated, total=%d bytes]", total)
	return full, total, preview, true
}

// BridgeIngestHandler HTTP 上报端点处理器（2026-08-05 HTTP-only 重构）
//
// mock 注入点（2026-08-05）：
//   - MockHandleIngress / MockPersistHistory 用于本地 mock 跑通 HTTP 长轮询 e2e，
//     不依赖 DB / Redis / AI 引擎。生产环境（NewBridgeIngestHandler）保持 nil，handler 走真实 ingress。
//   - 测试中通过 NewBridgeIngestHandlerWithMock 注入 fake，让 HandleIngressMessage / PersistBridgeHistory
//     走测试桩函数，验证 5min 去重、长轮询 reply 拉取、OutboundReplies 序列化等。
// v3 审计：新增 sseHandler 提供 SSE 流式响应（替换长轮询）
// Phase 1：新增 outboxFetcher 引用，支持后期注入 OutboxQuerier
type BridgeIngestHandler struct {
	ingress      *service.InboxIngressService
	mockHandle   func(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error)
	mockPersist  func(ctx context.Context, ev *model.MessageEvent, direction string) error
	leadMiner    func(ctx context.Context, ev *model.MessageEvent)
	sseHandler   *SSEHandler
	outboxFetcher *outboxDBFetcher
}

// NewBridgeIngestHandler 构造 HTTP ingest 处理器
func NewBridgeIngestHandler(ingress *service.InboxIngressService) *BridgeIngestHandler {
	h := &BridgeIngestHandler{ingress: ingress}
	h.outboxFetcher = &outboxDBFetcher{}
	h.sseHandler = NewSSEHandler(h.outboxFetcher)
	return h
}

// SetOutboxQuerier 注入 OutboxQuerier（由 router 在装配完成后调用一次）
func (h *BridgeIngestHandler) SetOutboxQuerier(q OutboxQuerier) {
	if h.outboxFetcher != nil {
		h.outboxFetcher.SetQuerier(q)
	}
}

// SetLeadMiner 设置线索挖掘回调（Douyin/TikTok 等群聊渠道用）
func (h *BridgeIngestHandler) SetLeadMiner(fn func(ctx context.Context, ev *model.MessageEvent)) {
	h.leadMiner = fn
}

// HandleOutboxSSE 桥接 outbox SSE 流式端点（替换长轮询）
//
// v3 审计：业界 Twilio Flex / Intercom 做法，长轮询 1-3s → SSE <500ms
// Phase 1：使用预初始化的 outboxFetcher（支持后期注入 OutboxQuerier）
func (h *BridgeIngestHandler) HandleOutboxSSE(c *gin.Context) {
	if h.sseHandler == nil {
		if h.outboxFetcher == nil {
			h.outboxFetcher = &outboxDBFetcher{}
		}
		h.sseHandler = NewSSEHandler(h.outboxFetcher)
	}
	h.sseHandler.HandleOutboxSSE(c)
}

// OutboxQuerier 由 repository.MessageHubRepository 实现的最小接口
//
// 解耦原因：bridge 包不能直接 import repository（避免与 service 形成循环依赖），
// 故在 bridge 内定义最小化接口，由 repository.MessageHubRepository 隐式实现。
type OutboxQuerier interface {
	FetchOutboundSince(ctx context.Context, channel, accountID string, sinceID uint64, limit int) ([]model.MessageHub, error)
}

// outboxDBFetcher 基于 message_hub.outbound 表的 fetcher
//
// Phase 1 实现：通过 OutboxQuerier 接口查询 DB，拉取 outbound 消息转换为 SSEEvent。
// querier 为 nil 时返回空列表（服务未初始化时的安全降级）。
type outboxDBFetcher struct {
	querier OutboxQuerier
}

// SetQuerier 后期注入 OutboxQuerier（避免装配阶段循环依赖；服务启动时由 router 调用一次）
func (f *outboxDBFetcher) SetQuerier(q OutboxQuerier) {
	f.querier = q
}

// FetchOutboxSince 查询 message_hub 表中 id > lastEventID 的 outbound 消息
func (f *outboxDBFetcher) FetchOutboxSince(ctx context.Context, channel, accountID, lastEventID string) ([]SSEEvent, string, error) {
	if f.querier == nil {
		return nil, "", nil
	}

	var sinceID uint64
	if lastEventID != "" {
		if id, err := strconv.ParseUint(lastEventID, 10, 64); err == nil {
			sinceID = id
		}
	}

	rows, err := f.querier.FetchOutboundSince(ctx, channel, accountID, sinceID, 200)
	if err != nil {
		return nil, "", err
	}

	events := make([]SSEEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, SSEEvent{
			ID:             strconv.FormatUint(uint64(row.ID), 10),
			Event:          "new_outbound",
			ConversationID: row.ConversationID,
			MsgType:        row.MsgType,
			ReceiverID:     row.ReceiverID,
			Seq:            int(row.ID),
			Data: map[string]any{
				"hub_id":          row.ID,
				"msg_id":          row.MsgID,
				"platform":        row.Platform,
				"account_id":      row.AccountID,
				"conversation_id": row.ConversationID,
				"content":         row.Content,
				"msg_type":        row.MsgType,
				"receiver_id":     row.ReceiverID,
				"is_ai_reply":     row.IsAIReply,
				"extra":           row.Extra,
			},
			Timestamp: row.CreatedAt,
		})
	}

	newLastID := lastEventID
	if len(rows) > 0 {
		newLastID = strconv.FormatUint(uint64(rows[len(rows)-1].ID), 10)
	}
	return events, newLastID, nil
}

// NewBridgeIngestHandlerWithMock 构造带 mock 的 HTTP ingest 处理器（仅测试用）
//
// mockHandle / mockPersist 任一为 nil 时，对应路径回退到真实 ingress（若 ingress 也 nil 则 panic）。
// 用于本地跑通 HTTP 长轮询 e2e，无需 DB/Redis/AI 引擎。
func NewBridgeIngestHandlerWithMock(
	mockHandle func(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error),
	mockPersist func(ctx context.Context, ev *model.MessageEvent, direction string) error,
) *BridgeIngestHandler {
	return &BridgeIngestHandler{mockHandle: mockHandle, mockPersist: mockPersist}
}



// callPersistHistory 走 mock 优先，否则真实 ingress
func (h *BridgeIngestHandler) callPersistHistory(ctx context.Context, ev *model.MessageEvent, direction string) error {
	if h.mockPersist != nil {
		return h.mockPersist(ctx, ev, direction)
	}
	return h.ingress.PersistBridgeHistory(ctx, ev, direction)
}

// HandleHTTPIngest POST /api/bridge/ingest 统一收件箱 HTTP 上报端点
//
// 2026-08-05 架构重构（用户诉求）：
//  1. 接收扩展一次性上报的多条消息
//  2. 对每条消息走 InboxIngressService.HandleIngressMessage（含 sender_type 过滤、
//     内容 hash 去重、5min 回复窗口、AI 触发）
//  3. 若 expect_reply=true 且至少一条消息触发 AI：长轮询等待 AI 推理完成（最多 HTTPPollingMaxTimeout）
//  4. 返回 ingest 处理结果 + outbound_replies（AI 回复）
//
// 鉴权（与 WS 端点一致）：
//   - 路由层仅过 InitGuard（系统须已初始化），不过 JWTAuthMiddleware
//   - 账号以 channel+account_id 自证身份（私有化部署单用户场景）
//   - 若请求携带有效 JWT，则再校验 (channel, account_id) 是否属于该 user
func (h *BridgeIngestHandler) HandleHTTPIngest(c *gin.Context) {
	info := collectHTTPRequestInfo(c)
	channel := info.Channel
	accountID := info.AccountID
	conversationID := info.ConversationID
	ctx0 := c.Request.Context()
	start := time.Now()
	bm := metrics.GetBridge()

	var req HTTPIngestRequest
	bodyBindErr := c.ShouldBindJSON(&req)
	if bodyBindErr == nil {
		if channel == "" {
			channel = req.Channel
		}
		if accountID == "" {
			accountID = req.AccountID
		}
		if conversationID == "" {
			conversationID = req.ConversationID
		}
	}

	channelNorm := NormalizeBridgeChannel(channel)

	bridgeHTTPIngestError := func(errCode string) {
		bm.IngestErrors.WithLabel(channel, errCode).Inc()
		bm.IngestDuration.WithLabel(channel).Observe(float64(time.Since(start).Milliseconds()))
	}

	logger.Ctx(ctx0).Info().
		Str("module", "bridge").
		Str("event", "http_ingest_request").
		Str("full_url", info.FullURL).
		Str("method", info.Method).
		Str("path", info.Path).
		Str("raw_query", info.RawQuery).
		Str("content_type", info.ContentType).
		Int64("content_length", info.ContentLength).
		Str("channel", channel).
		Str("channel_norm", channelNorm).
		Str("account_id", accountID).
		Str("conversation_id", conversationID).
		Str("token", info.TokenMasked).
		Str("remote_addr", info.RemoteAddr).
		Str("origin", info.Origin).
		Str("user_agent", info.UserAgent).
		Interface("parsed_query", info.ParsedQuery).
		Str("body_preview", info.BodyPreview).
		Int("body_size", info.BodySize).
		Msg("[Bridge HTTP] 收到 ingest 请求（完整 URL + 全部参数 + body 预览）")

	if channel == "" {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_missing_params").
			Str("full_url", info.FullURL).
			Str("channel", channel).
			Str("account_id", accountID).
			Msg("[Bridge HTTP] 参数缺失：channel 为空")
		bridgeHTTPIngestError("channel_required")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "channel required",
		})
		return
	}
	if accountID == "" {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_missing_params").
			Str("full_url", info.FullURL).
			Str("channel", channel).
			Str("account_id", accountID).
			Msg("[Bridge HTTP] 参数缺失：account_id 为空（前端必须取到账号才能上报）")
		bridgeHTTPIngestError("account_id_required")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "account_id required (extension must capture account from DOM before ingesting)",
		})
		return
	}
	if !IsBridgeChannel(channel) {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_unsupported_channel").
			Str("full_url", info.FullURL).
			Str("channel", channel).
			Str("account_id", accountID).
			Msg("[Bridge HTTP] 渠道不在白名单")
		bridgeHTTPIngestError("unsupported_channel")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "unsupported bridge channel",
		})
		return
	}

	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	if GlobalOwnershipChecker != nil && uid != 0 {
		owns, err := GlobalOwnershipChecker(c.Request.Context(), uid, channel, accountID)
		if err != nil {
			logger.Ctx(ctx0).Warn().
				Str("module", "bridge").
				Str("event", "http_ingest_ownership_check_failed").
				Err(err).
				Str("full_url", info.FullURL).
				Str("channel", channel).
				Str("account_id", accountID).
				Uint("user_id", uid).
				Msg("[Bridge HTTP] 账号归属校验失败")
			c.JSON(http.StatusInternalServerError, HTTPIngestResponse{
				OK:     false,
				Reason: "ownership check failed: " + err.Error(),
			})
			return
		}
		if !owns {
			logger.Ctx(ctx0).Warn().
				Str("module", "bridge").
				Str("event", "http_ingest_ownership_denied").
				Str("full_url", info.FullURL).
				Str("channel", channel).
				Str("account_id", accountID).
				Uint("user_id", uid).
				Msg("[Bridge HTTP] 账号归属不属于当前 user，拒绝 ingest")
			c.JSON(http.StatusForbidden, HTTPIngestResponse{
				OK:     false,
				Reason: "account not owned by current user",
			})
			return
		}
	}

	if info.ContentLength > HTTPIngestMaxBodySize {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_body_too_large").
			Str("full_url", info.FullURL).
			Int64("content_length", info.ContentLength).
			Msg("[Bridge HTTP] body 超过 4MB 上限")
		c.JSON(http.StatusRequestEntityTooLarge, HTTPIngestResponse{
			OK:     false,
			Reason: fmt.Sprintf("body too large (max %d bytes)", HTTPIngestMaxBodySize),
		})
		return
	}

	if bodyBindErr != nil && info.ContentLength > 0 {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_bind_json_failed").
			Err(bodyBindErr).
			Str("full_url", info.FullURL).
			Msg("[Bridge HTTP] body JSON 解析失败")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "invalid json body: " + bodyBindErr.Error(),
		})
		return
	}

	if channel != "" {
		req.Channel = channel
	}
	if accountID != "" {
		req.AccountID = accountID
	}
	if conversationID != "" {
		req.ConversationID = conversationID
	}
	if len(req.Messages) > HTTPIngestMaxMessages {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_truncated").
			Str("full_url", info.FullURL).
			Int("messages", len(req.Messages)).
			Int("truncated_to", HTTPIngestMaxMessages).
			Msg("[Bridge HTTP] 消息数超过 200 上限，截断处理（剩余由下轮巡检补齐）")
		req.Messages = req.Messages[:HTTPIngestMaxMessages]
	}

	traceID := c.GetHeader("X-Trace-Id")
	if traceID == "" {
		if v, ok := c.Get("trace_id"); ok {
			traceID, _ = v.(string)
		}
	}
	ctx := c.Request.Context()
	if traceID != "" {
		ctx = logger.WithTraceID(ctx, traceID)
	} else {
		ctx = logger.WithTraceID(ctx, "")
	}
	ctx = logger.WithModule(ctx, "bridge")

	if GlobalBridgeAccountRepo != nil && (req.AgentID > 0 || req.AccountName != "" || len(req.Messages) > 0) {
		up := BridgeAccountUpsert{
			UserID:      uid,
			Channel:     NormalizeBridgeChannel(channel),
			AccountID:   accountID,
			AgentID:     req.AgentID,
			AccountName: req.AccountName,
			Status:      "online",
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Ctx(context.Background()).Error().
						Interface("panic", r).
						Str("module", "bridge").
						Str("channel", up.Channel).
						Str("account_id", up.AccountID).
						Msg("[Bridge] async HTTP Upsert panic recovered")
				}
			}()
			upCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := GlobalBridgeAccountRepo.Upsert(upCtx, up); err != nil {
				if !errors.Is(err, ErrAccountOwnedByOther) {
					logger.Ctx(context.Background()).Warn().Err(err).Str("module", "bridge").
						Str("channel", up.Channel).Str("account_id", up.AccountID).
						Msg("bridge HTTP Upsert failed (non-fatal)")
				}
			}
		}()
	}

	resp := &HTTPIngestResponse{
		OK:         true,
		Ingested:   make([]*HTTPIngestResult, 0, len(req.Messages)),
		ServerTime: time.Now().UnixMilli(),
	}

	// 2026-08-05 重构（用户科学方案）：
	//   - 逐条消息预处理：self/agent 走历史通道，customer + history 先持久化上下文
	//   - 收集所有需走 ingress 的 customer 消息，统一调用 HandleIngressBatch
	//   - batch 内按 conversation 分组 + 逐条 msg_id 去重入库 + 时序锚点判断
	//   - batch 末尾合并 inbound 消息一次 AI 回复（不无限制给用户发消息）
	var batchEvents []*model.MessageEvent
	batchIdxMap := make(map[int]int) 

	for i, m := range req.Messages {
		if m == nil {
			continue
		}
		if m.Channel == "" {
			m.Channel = req.Channel
		}
		if m.AccountID == "" {
			m.AccountID = req.AccountID
		}
		if m.ConversationID == "" {
			m.ConversationID = req.ConversationID
		}
		if len(m.History) > 0 {
			liveEventID := m.EventID
			for hi, it := range m.History {
				if it == nil {
					continue
				}
				if liveEventID != "" && it.EventID == liveEventID {
					continue
				}
				item := historyItemToEvent(httpMessageToUnified(m), it)
				histDir := it.Direction
				if histDir == "" {
					histDir = "inbound"
				}
				if (m.SenderType == "self" || m.SenderType == "agent") && histDir != "outbound" {
					histDir = "outbound"
				}
				if err := h.callPersistHistory(ctx, item, histDir); err != nil {
					logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").
						Str("event_id", it.EventID).Str("conv_id", m.ConversationID).
						Int("history_idx", hi).
						Msg("[Bridge HTTP] history context persist failed")
				}
			}
		}
		ev := httpMessageToEvent(m)
		if req.InternalOnly {
			if err := h.callPersistHistory(ctx, ev, "inbound"); err != nil {
				logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").
					Str("event_id", m.EventID).Str("conv_id", m.ConversationID).
					Msg("[Bridge HTTP] internal_only persist failed")
			}
			resp.Ingested = append(resp.Ingested, &HTTPIngestResult{
				EventID:  m.EventID,
				Accepted: true,
				Reason:   "internal_only: persisted only",
			})
			continue
		}
		batchIdxMap[i] = len(batchEvents)
		batchEvents = append(batchEvents, ev)
	}

	anyAIQueued := false
	if len(batchEvents) > 0 {
		batchResult, err := h.callHandleIngressBatch(ctx, batchEvents)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").
				Msg("[Bridge HTTP] HandleIngressBatch failed")
		}
		if batchResult != nil {
			for origIdx, batchIdx := range batchIdxMap {
				if batchIdx >= len(batchResult.PerEvent) {
					continue
				}
				result := batchResult.PerEvent[batchIdx]
				if result == nil {
					continue
				}
				m := req.Messages[origIdx]
				r := &HTTPIngestResult{
					EventID:   m.EventID,
					Accepted:  result.Accepted,
					AIHandled: result.QueuedForAI,
					Reason:    result.Reason,
				}
				if isIngestDuplicate(result.Reason) {
					r.Duplicate = true
				}
				if result.SessionID != "" {
					resp.SessionID = result.SessionID
				}
				resp.Ingested = append(resp.Ingested, r)
				if result.QueuedForAI {
					anyAIQueued = true
				}
			}
			_ = batchResult.TriggeredAI
		}
	}

	if h.leadMiner != nil && isLeadMiningChannel(channelNorm) {
		for _, ev := range batchEvents {
			h.leadMiner(ctx, ev)
		}
	}

	ingestedCount := len(resp.Ingested)
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_ingest_response").
		Str("full_url", info.FullURL).
		Str("channel", channelNorm).
		Str("channel_raw", channel).
		Str("account_id", accountID).
		Str("conv_id", conversationID).
		Int("ingested_count", ingestedCount).
		Int("any_ai_queued", boolToInt(anyAIQueued)).
		Int64("server_time_ms", resp.ServerTime).
		Msg("[Bridge HTTP] ingest 响应（每条结果）")

	bm.IngestTotal.WithLabel(channelNorm, strconv.FormatUint(uint64(req.AgentID), 10)).Inc()
	bm.IngestDuration.WithLabel(channelNorm).Observe(float64(time.Since(start).Milliseconds()))

	c.JSON(http.StatusOK, resp)
}


// BridgeOutboxMessage 下发队列中的一条待发消息（别名 channelgw.OutboxMessage，HTTP/WS 共用序列化）。
type BridgeOutboxMessage = channelgw.OutboxMessage

// BridgeOutboxAckItem v2 协议单条 ack 条目（P0-1）。
//
// 2026-08-15 P0-1：v2 协议下每条 item 必须带 conversation_id，服务端按
// (channel, account_id, msg_id, conversation_id) 严格去重，根除"跨会话同名 msg_id 一锅端"。
type BridgeOutboxAckItem struct {
	MsgID          string `json:"msg_id"`
	ConversationID string `json:"conversation_id"`
	Status         string `json:"status,omitempty"`
	Error          string `json:"error,omitempty"`
}

// BridgeOutboxAckRequest 状态上报请求体（通道B）。
//
// 2026-08-15 P0-1 协议升级：
//   - v1（缺省）：{ msg_ids: [...], status: "delivered" }，按 (channel, account_id) 范围 ack（兼容旧扩展）
//   - v2：{ v: 2, items: [{ msg_id, conversation_id, status?, error? }], status? }，
//     每个 item 必填 conversation_id，缺则 400；按 (channel, account_id, msg_id, conversation_id) 严格去重
type BridgeOutboxAckRequest struct {
	V      int                   `json:"v,omitempty"`
	MsgIDs []string              `json:"msg_ids"`
	Status string                `json:"status"`
	Items  []BridgeOutboxAckItem `json:"items"`
}

// GetBridgeOutbox 桥接下发队列查询（通道C·下发轮询）。
// 扩展独立轮询此端点，拉取本渠道/账号下 status='pending' 的出站消息（AI 回复），
// 转发到对应网页会话，成功后通过 AckBridgeOutbox 确认。
//
// writeOutboxJSON 把 message_hub 列表序列化应答（通道C·下发）。
//
// 2026-08-14 修复：补 Extra 字段
//   协议新增 Extra 后，必须从 hub.Extra 透传，否则前端 downlink.js 主动私信路由永远拿不到数据。
func writeOutboxJSON(c *gin.Context, hubs []*model.MessageHub) {
	msgs := make([]BridgeOutboxMessage, 0, len(hubs))
	for _, hub := range hubs {
		msgs = append(msgs, BridgeOutboxMessage{
			MsgID:          hub.MsgID,
			ConversationID: hub.ConversationID,
			MsgType:        hub.MsgType,
			Content:        hub.Content,
			MediaURL:       hub.MediaURL,
			SenderID:       hub.SenderID,
			ReceiverID:     hub.ReceiverID,
			IsAIReply:      hub.IsAIReply,
			CreatedAt:      hub.CreatedAt,
			Extra:          hub.Extra,
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "messages": msgs})
}

// GET /api/bridge/outbox?channel=<ch>&account_id=<acc>
// isIngestDuplicate 委托 channelgw.IsDuplicateReason（HTTP/WS 传输共用同一判定）。
func isIngestDuplicate(reason string) bool {
	return channelgw.IsDuplicateReason(reason)
}

// 2026-08-16 重大修正：白名单只保留有真实 Chrome 扩展/桥接客户端支撑的渠道
//
//	真实 Bridge 渠道：douyin / tiktok / kuaishou / xiaohongshu / xianyu
//	（有 Chrome 扩展 + Bridge 协议对齐 + channelgw 注册）
//
//	未实现渠道（移除出白名单）：
//	  weibo / bilibili / taobao / pdd / jd / 1688
//	  — 这些是上次论证书面"支持"但实际并无适配代码、Chrome 扩展、channelgw 注册，
//	    仍将其放进 leadMiningChannels 只会让线索挖掘把假渠道当真渠道去解析。
//	如需新增渠道：必须先实现 Chrome 扩展 manifest / content script / 与 Bridge 协议对齐，
//	并在 channelgw.Default 注册 ChannelSpec（含 Label/Transports），
//	再调 AddLeadMiningChannel 加回白名单。
var leadMiningChannels = map[string]bool{
	"douyin":      true,
	"tiktok":      true,
	"kuaishou":    true,
	"xiaohongshu": true,
	"xianyu":      true,
}

func isLeadMiningChannel(channel string) bool {
	return leadMiningChannels[strings.ToLower(channel)]
}

// AddLeadMiningChannel 动态扩展支持群聊线索挖掘的 Bridge 渠道（插件使用）。
//
//	使用约束：调用方必须先在 channelgw.Default 注册同名 ChannelSpec，
//	且必须有对应的 Chrome 扩展 content script 与 Bridge 协议对齐，
//	否则 ingest 会被 IsBridgeChannel 拒绝。
func AddLeadMiningChannel(channel string) {
	leadMiningChannels[strings.ToLower(channel)] = true
}

// _bridgeOutboxHubsForLog 把 hubs 转成日志友好的可序列化结构。
// 2026-08-14 用户诉求：下发响应完整打 log（content 不截断），便于对照前端 getOutbox 排查"拉到了什么下发"。
func _bridgeOutboxHubsForLog(hubs []*model.MessageHub) []map[string]any {
	out := make([]map[string]any, 0, len(hubs))
	for _, h := range hubs {
		if h == nil {
			continue
		}
		out = append(out, map[string]any{
			"msg_id":          h.MsgID,
			"conversation_id": h.ConversationID,
			"platform":        h.Platform,
			"account_id":      h.AccountID,
			"sender_id":       h.SenderID,
			"receiver_id":     h.ReceiverID,
			"msg_type":        h.MsgType,
			"content":         h.Content,
			"media_url":       h.MediaURL,
			"is_ai_reply":     h.IsAIReply,
			"status":          h.Status,
			"sent_at":         h.SentAt,
			"created_at":      h.CreatedAt,
			"extra":           h.Extra,
		})
	}
	return out
}

func (h *BridgeIngestHandler) GetBridgeOutbox(c *gin.Context) {
	channel := c.Query("channel")
	accountID := c.Query("account_id")
	start := time.Now()
	bm := metrics.GetBridge()
	// bridgeOutboxError 记录 outbox 校验错误指标（复用 ingest_errors 分类）。
	bridgeOutboxError := func(errCode string) {
		bm.IngestErrors.WithLabel(channel, errCode).Inc()
	}
	if accountID == "" {
		bridgeOutboxError("account_id_required")
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "account_id required (extension must capture account from DOM before polling outbox)",
		})
		return
	}
	if channel == "" {
		bridgeOutboxError("channel_required")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "channel required"})
		return
	}
	if !IsBridgeChannel(channel) {
		bridgeOutboxError("unsupported_channel")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "unsupported bridge channel"})
		return
	}
	ctx := c.Request.Context()
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_request").
		Str("full_url", c.Request.URL.String()).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Str("raw_query", c.Request.URL.RawQuery).
		Str("channel", channel).
		Str("account_id", accountID).
		Str("remote_addr", c.Request.RemoteAddr).
		Interface("parsed_query", describeUpstreamQuery(c.Request.URL.Query())).
		Msg("[Bridge HTTP] 收到 outbox 请求（完整 URL + 全部参数）")
	// 解析 limit：默认 50，上限 200。一次性解析避免与 fetch/log/serialize 路径重复。
	limit := 50
	if q := c.Query("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	hubs, err := h.ingress.ClaimPendingOutbound(ctx, channel, accountID, limit)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("module", "bridge").Str("channel", channel).Msg("[Bridge] ListPendingOutbound failed")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "query outbox failed"})
		return
	}
	tracing.RecordDownlinkFetchBatch(ctx, channel, accountID, hubs)
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_response").
		Str("full_url", c.Request.URL.String()).
		Str("channel", channel).
		Str("account_id", accountID).
		Int("limit", limit).
		Int("messages_count", len(hubs)).
		Interface("messages", _bridgeOutboxHubsForLog(hubs)).
		Msg("[Bridge HTTP] outbox 响应（完整待发消息列表，含 content）")
	bm.OutboxFetched.WithLabel(channel, accountID).Add(uint64(len(hubs)))
	bm.OutboxDuration.WithLabel(channel).Observe(float64(time.Since(start).Milliseconds()))
	writeOutboxJSON(c, hubs)
}

// BridgeOutboxAckResponse 状态确认响应（通道B），含 per-msg-id 详细状态（P3-D 2026-08-15 + P4 二次审核 6.2）。
//
// 字段语义（2026-08-15 P4 区分 affected vs acked_items）：
//   - AffectedCount:     SQL UPDATE 实际翻转为 delivered 的行数（跨会话同名 msg_id 时可能 > AckedItemsCount）
//   - AckedItemsCount:   items 中 status='acked' 的元素数（= 真正"被本次 ack 命中"的 msg_id 数）
//   - DuplicateCount:    此前已为 delivered 的 msg_id 数（幂等跳过）
//   - NotFoundCount:     不存在的 msg_id 数
//   - Items:             按入参 msg_ids 顺序（去重后）逐条结果
//
// 协议契约（与前端 downlink.js 配套）：
//   - items[].status = "acked"        本次成功翻转 pending→delivered
//   - items[].status = "duplicate"    此前已为 delivered，本地重试队列可清空
//   - items[].status = "not_found"    本 (channel, account_id) 下不存在，停止重发
type BridgeOutboxAckResponse struct {
	Status           string                    `json:"status"`
	AffectedCount    int                       `json:"affected_count"`     
	AckedItemsCount  int                       `json:"acked_items_count"`  
	FailedItemsCount int                       `json:"failed_items_count"`
	DuplicateCount   int                       `json:"duplicate_count"`
	NotFoundCount    int                       `json:"not_found_count"`
	NotInScopeCount  int                       `json:"not_in_scope_count"`
	Items            []service.AckOutboundItem `json:"items"` 
}

// AckBridgeOutbox 桥接下发状态确认（通道B·状态上报）。
// 扩展把消息成功转发到网页后，批量上报 msg_ids，服务端标记为 delivered。
//
// POST /api/bridge/outbox/ack  body: {"msg_ids":[...],"status":"delivered"}
//
// 2026-08-15 P3-D：响应包含 per-msg-id 详细状态（acked/duplicate/not_found）。
// 2026-08-15 P4 二次审核修复：
//   - 7.3: 加 MaxBytesReader 1MB body 大小保护（防 DoS）
//   - 6.2: 区分 affected_count（行级）与 acked_items_count（msg_id 级）
//   - 3.3: 去掉 items omitempty 始终输出数组
func (h *BridgeIngestHandler) AckBridgeOutbox(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	channel := c.Query("channel")
	accountID := c.Query("account_id")
	start := time.Now()
	bm := metrics.GetBridge()
	if accountID == "" {
		bm.IngestErrors.WithLabel(channel, "account_id_required").Inc()
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "account_id required (extension must capture account from DOM before acking)",
		})
		return
	}
	if channel == "" {
		bm.IngestErrors.WithLabel(channel, "channel_required").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "channel required"})
		return
	}
	if !IsBridgeChannel(channel) {
		bm.IngestErrors.WithLabel(channel, "unsupported_channel").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "unsupported bridge channel"})
		return
	}
	var req BridgeOutboxAckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid body"})
		return
	}
	// 2026-08-15 P3-D：单次 ack 上限（防止前端误传超长列表）
	const maxAckMsgIDs = 500
	if len(req.MsgIDs) > maxAckMsgIDs {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("too many msg_ids (max %d per request)", maxAckMsgIDs),
		})
		return
	}
	ctx := c.Request.Context()
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_ack_request").
		Str("full_url", c.Request.URL.String()).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Str("raw_query", c.Request.URL.RawQuery).
		Str("channel", channel).
		Str("account_id", accountID).
		Str("remote_addr", c.Request.RemoteAddr).
		Interface("parsed_query", describeUpstreamQuery(c.Request.URL.Query())).
		Str("body_status", req.Status).
		Int("body_msg_ids_count", len(req.MsgIDs)).
		Interface("body_msg_ids", req.MsgIDs).
		Msg("[Bridge HTTP] 收到 outbox ack 请求（完整 URL + 全部参数 + 完整 msg_ids 列表）")
	// 2026-08-15 P3-D：terminalStatus 透传（默认 delivered；前端可上报 failed），
	// conversationID 留空走 v1 兼容范围（按 channel+account_id ack），perItem 无（旧协议无 items[]）。
	terminalStatus := req.Status
	if terminalStatus == "" {
		terminalStatus = model.BridgeAckStatusDelivered
	}
	ackResult, err := h.ingress.AckOutboundDeliveredDetailed(ctx, channel, accountID, req.MsgIDs, "", terminalStatus, nil)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("module", "bridge").Str("channel", channel).Msg("[Bridge] AckOutboundDeliveredDetailed failed")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "ack failed"})
		return
	}
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_ack_response").
		Str("full_url", c.Request.URL.String()).
		Str("channel", channel).
		Str("account_id", accountID).
		Int("affected_count", ackResult.AffectedCount).
		Int("acked_items_count", ackResult.AckedItemsCount).
		Int("duplicate_count", ackResult.DuplicateCount).
		Int("not_found_count", ackResult.NotFoundCount).
		Int("request_msg_ids_count", len(req.MsgIDs)).
		Interface("request_msg_ids", req.MsgIDs).
		Interface("ack_items", ackResult.Items).
		Msg("[Bridge HTTP] outbox ack 响应（行级 affected + msg_id 级 acked + 详细 ack 结果）")
	for _, item := range ackResult.Items {
		if item.Status == "" {
			continue
		}
		bm.OutboxAcked.WithLabel(channel, item.Status).Inc()
	}
	bm.AckDuration.WithLabel(channel).Observe(float64(time.Since(start).Milliseconds()))
	c.JSON(http.StatusOK, BridgeOutboxAckResponse{
		Status:          "ok",
		AffectedCount:   ackResult.AffectedCount,
		AckedItemsCount: ackResult.AckedItemsCount,
		DuplicateCount:  ackResult.DuplicateCount,
		NotFoundCount:   ackResult.NotFoundCount,
		Items:           ackResult.Items,
	})
}

// callHandleIngressBatch 走 mock 优先，否则真实 ingress 的 HandleIngressBatch
func (h *BridgeIngestHandler) callHandleIngressBatch(ctx context.Context, events []*model.MessageEvent) (*service.InboxIngressBatchResult, error) {
	if h.mockHandle != nil {
		results := make([]*service.InboxIngressResult, len(events))
		triggered := false
		for i, ev := range events {
			r, err := h.mockHandle(ctx, ev)
			if err != nil {
				r = &service.InboxIngressResult{Reason: err.Error()}
			}
			results[i] = r
			if r.QueuedForAI {
				triggered = true
			}
		}
		return &service.InboxIngressBatchResult{PerEvent: results, TriggeredAI: triggered}, nil
	}
	if h.ingress == nil {
		return nil, errors.New("ingress service not configured")
	}
	return h.ingress.HandleIngressBatch(ctx, events)
}

// httpMessageToEvent 将 HTTP 单条消息转 model.MessageEvent（委托 channelgw 规范化转换器，
// 与 WS 传输同源；transport 标记 "http" 写入 Extra 供可观测）。
func httpMessageToEvent(m *HTTPIngestMessage) *model.MessageEvent {
	if m == nil {
		return nil
	}
	m.Channel = NormalizeBridgeChannel(m.Channel)
	return m.ToEvent("http")
}

// httpMessageToUnified HTTP 单条消息 → UnifiedMessage（仅用于 historyItemToEvent 提取 channel/account/conversation）。
func httpMessageToUnified(m *HTTPIngestMessage) *UnifiedMessage {
	return &UnifiedMessage{
		Channel:        m.Channel,
		AccountID:      m.AccountID,
		ConversationID: m.ConversationID,
		ReceiverID:     m.ReceiverID,
		SenderType:     m.SenderType,
		IsGroup:        m.IsGroup,
		GroupID:        m.GroupID,
		GroupName:      m.GroupName,
	}
}

// boolToInt 辅助：false→0, true→1（zap zerolog Field 不接受 bool）
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// 编译期断言（防止 import 被优化掉）
var _ = url.Values{}
var _ = bytes.NewBuffer

