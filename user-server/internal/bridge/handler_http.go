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
	"sync"
	"time"

	"hivemtk-user/internal/channelgw"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// =============================================================
// HTTP 长轮询上报端点（POST /api/bridge/ingest）
//
// 2026-08-05 架构重构（用户诉求）：
//   - bridge 模块不再维护 WebSocket 长连接，改用 HTTP 长轮询
//   - bridge 端：每秒巡检一次会话列表 → 进入会话抓多轮消息 → 一次性 POST
//   - 统一收件箱：去重（5min SHA-256 内容 hash）→ 落库 → 触发 AI → 返回回复
//   - 优势：架构简单（0 长连接、0 goroutine、0 重连状态机）、curl 可测、
//     天然 OOM-safe（HTTP 用过即释放）、MV3 SW 冻结不影响
//   - 配套：bridge 端日志含完整 URL + 全部 query + body；user-server 入口
//     日志含完整 URL + body + 响应结果，便于对照定位
//
// 与现有 WS 端点（/api/ws/bridge）并存：WS 标记为 deprecated，新扩展走 HTTP。
// =============================================================

// HTTP 端点参数（与 WS 端点默认值严格对齐，单源来自 DEFAULTS.md）
const (
	// HTTPPollingMaxTimeout 单次 HTTP 请求最大等待时间（与用户诉求 "500 秒超时" 对齐）
	HTTPPollingMaxTimeout = 500 * time.Second
	// HTTPPollingDefaultTimeout 默认超时（30s；扩展无显式指定时使用）
	HTTPPollingDefaultTimeout = 30 * time.Second
	// HTTPIngestMaxBodySize 单请求 body 最大字节数（防止恶意扩展灌大包）
	HTTPIngestMaxBodySize = 4 << 20 // 4MB
	// HTTPIngestMaxMessages 单请求最多消息条数
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
	mu      sync.Mutex
	waiters map[string]chan *HTTPIngestResponse // key: conversation_id
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
	BodyPreview    string // body 前 4KB 预览（截断避免日志撑爆）
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
	// 在写回 body 之前先快照原始 ContentLength，否则会被覆盖
	origContentLength := c.Request.ContentLength
	// 读取完整 body：下游 BindJSON 需要完整 JSON，不能只写回预览
	fullBody, bodySize, bodyPreview, _ := readBodyForLog(c.Request.Body, 4096)
	// 写回完整 body（非截断预览），避免 BindJSON 读到截断 JSON 报 unexpected EOF
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
type BridgeIngestHandler struct {
	ingress *service.InboxIngressService
	// mockHandle / mockPersist 测试用：nil 时走真实 ingress，否则走注入函数
	mockHandle  func(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error)
	mockPersist func(ctx context.Context, ev *model.MessageEvent, direction string) error
}

// NewBridgeIngestHandler 构造 HTTP ingest 处理器
func NewBridgeIngestHandler(ingress *service.InboxIngressService) *BridgeIngestHandler {
	return &BridgeIngestHandler{ingress: ingress}
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

// callHandleIngress 走 mock 优先，否则真实 ingress
func (h *BridgeIngestHandler) callHandleIngress(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error) {
	if h.mockHandle != nil {
		return h.mockHandle(ctx, ev)
	}
	return h.ingress.HandleIngressMessage(ctx, ev)
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
	// 2026-08-05 重构：所有 HTTP ingest 日志共用同一 httpRequestInfo 快照。
	//   1) 收集完整 URL + 全部 query + 来源信息（含 token 脱敏）
	//   2) 5 个日志点统一格式（收到请求 / 参数缺失 / 渠道拒绝 / ingest 处理 / 响应回写）
	//   3) 与扩展端 http-ingest.js _logRequest 输出对照即可定位参数丢失/错传问题
	info := collectHTTPRequestInfo(c)
	channel := info.Channel
	accountID := info.AccountID
	conversationID := info.ConversationID
	// 归一化渠道（2026-08-13 修复）：扩展端可能上报 xhs / xhs_web 等历史简写，
	// 落库 message_hub.platform 已统一为全名，日志此处同步展示归一化值，
	// 便于"分析上报日志"时与 DB 关联（原始值仍保留在 channel_raw 字段）。
	channelNorm := NormalizeBridgeChannel(channel)
	ctx0 := c.Request.Context()

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

	// 必填参数校验（2026-08-14 治本）：channel 必填，account_id 也必填。
	// 2026-08-14 修复：account_id 不再兜底为 "default"
	//   原实现：account_id 为空 → accountID = "default" → 所有"未抓到账号"的扩展消息混到一起，
	//            污染 message_hub.account_id 维度的聚合（按 account 限速/分组全错）。
	//   新实现：channel 必填；account_id 也必填（前端 types.js 已不发送空字符串；
	//            若前端退化/扩展 bug 漏字段，后端必须立即拒绝，而不是写 "default" 兜底）。
	//   理由：兜底"垃圾"account_id 是私域部署绝对禁止的——单租户强隔离要求
	//            "所有数据来源可追溯"，"default" 静默收容所有抓不到账号的脏消息。
	if channel == "" {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_missing_params").
			Str("full_url", info.FullURL).
			Str("channel", channel).
			Str("account_id", accountID).
			Msg("[Bridge HTTP] 参数缺失：channel 为空")
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
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "account_id required (extension must capture account from DOM before ingesting)",
		})
		return
	}
	// 渠道白名单
	if !IsBridgeChannel(channel) {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_unsupported_channel").
			Str("full_url", info.FullURL).
			Str("channel", channel).
			Str("account_id", accountID).
			Msg("[Bridge HTTP] 渠道不在白名单")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "unsupported bridge channel",
		})
		return
	}

	// 解析 JWT（可选）：与 WS 端一致，无 JWT 时归属写入 user_id=0
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

	// 解析 body（collectHTTPRequestInfo 已写回 body，可直接 BindJSON）
	// Body 大小保护：info.ContentLength 是原始 ContentLength（collect 会写回 body 后修改 c.Request.ContentLength）
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
	var req HTTPIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_bind_json_failed").
			Err(err).
			Str("full_url", info.FullURL).
			Msg("[Bridge HTTP] body JSON 解析失败")
		c.JSON(http.StatusBadRequest, HTTPIngestResponse{
			OK:     false,
			Reason: "invalid json body: " + err.Error(),
		})
		return
	}
	// query 始终优先于 body（防扩展错传：body 内 channel/account_id 若与 query 冲突，
	// 以 URL query 为准；query 缺失时才回退用 body 值，保持兼容）。
	// 注意：之前实现是 body 优先（req=="" 才用 query），与注释意图相反，已是 bug。
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
		// 2026-08-05 修复：超 200 条不再拒绝整批，改为截断到 200 条并告警
		//   拒绝整批会导致消息丢失（bridge 不会重发），截断保证前 200 条入库
		//   剩余消息由 bridge 下一轮巡检补齐（msg_id 稳定 → 幂等去重）
		logger.Ctx(ctx0).Warn().
			Str("module", "bridge").
			Str("event", "http_ingest_truncated").
			Str("full_url", info.FullURL).
			Int("messages", len(req.Messages)).
			Int("truncated_to", HTTPIngestMaxMessages).
			Msg("[Bridge HTTP] 消息数超过 200 上限，截断处理（剩余由下轮巡检补齐）")
		req.Messages = req.Messages[:HTTPIngestMaxMessages]
	}

	// trace_id 透传
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

	// 异步 upsert 账号（注册态），不阻塞主路径
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

	// 处理每条消息 + 收集回复
	resp := &HTTPIngestResponse{
		OK:         true,
		Ingested:   make([]*HTTPIngestResult, 0, len(req.Messages)),
		ServerTime: time.Now().UnixMilli(),
	}
	outboundReplies := make([]*UnifiedReply, 0)

	// 2026-08-05 重构（用户科学方案）：
	//   - 逐条消息预处理：self/agent 走历史通道，customer + history 先持久化上下文
	//   - 收集所有需走 ingress 的 customer 消息，统一调用 HandleIngressBatch
	//   - batch 内按 conversation 分组 + 逐条 msg_id 去重入库 + 时序锚点判断
	//   - batch 末尾合并 inbound 消息一次 AI 回复（不无限制给用户发消息）
	var batchEvents []*model.MessageEvent
	batchIdxMap := make(map[int]int) // 原消息索引 -> batchEvents 索引

	for i, m := range req.Messages {
		if m == nil {
			continue
		}
		// 兜底：消息内 channel/account_id/conversation_id 与请求级一致
		if m.Channel == "" {
			m.Channel = req.Channel
		}
		if m.AccountID == "" {
			m.AccountID = req.AccountID
		}
		if m.ConversationID == "" {
			m.ConversationID = req.ConversationID
		}
		// 历史上下文回填（仅落库，不影响实时自/他权威判定）
		// 平台自己(self/agent)的历史项强制 outbound，其余按前端 direction（默认 inbound）。
		if len(m.History) > 0 {
			// 修复（2026-08-06）：实时消息本身不得作为 history 回填。
			// 前端会把当前正在上报的实时消息也塞进 history 数组（相同 EventID），
			// 若先经 callPersistHistory 落库相同 msg_id，则后续 batch 处理实时消息时
			// GetByMsgID 命中"已存在"→ 钩子2 幂等跳过 QueuedForAI=false →
			// newInboundContents 为空 → AI 永不触发（会话 2268 实测：用户新消息入库但无 AI 回复）。
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
		// 实时消息统一走 ingress：服务端按"内容是否命中平台下发 outbound"权威判定
		// 自/他（前端 sender_type 不可信，见 InboxIngressService.isPlatformOutboundEcho），
		// 不再在此预路由 self/agent。
		ev := httpMessageToEvent(m)
		// internal_only：仅落库不触发 AI
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
		// 收集到 batch（统一走 HandleIngressBatch 合并处理）
		batchIdxMap[i] = len(batchEvents)
		batchEvents = append(batchEvents, ev)
	}

	// 批量处理（按 conversation 分组 + msg_id 去重 + 时序锚点 + batch 内合并 AI 回复）
	anyAIQueued := false
	if len(batchEvents) > 0 {
		batchResult, err := h.callHandleIngressBatch(ctx, batchEvents)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").
				Msg("[Bridge HTTP] HandleIngressBatch failed")
		}
		// 将 batch 结果回填到每条消息（保持索引对齐）
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
				// 上报 ack 显式闭环：命中「幂等跳过 / 中间件拦截（回环回显 / 短时重复）」均标记 Duplicate，
				// 前端据此关闭该 event_id 的重发计时器，不再重复上报（允许重复上报，但服务端用 ack 确认去重）。
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

	// 2026-08-06 架构重构：ingest 即时返回，不再长轮询 AI 回复。
	// AI 回复落 message_hub(status=pending) 后，由扩展独立轮询 GET /api/bridge/outbox 拉取下发。
	resp.OutboundReplies = outboundReplies

	// 响应日志
	replyCount := len(resp.OutboundReplies)
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
		Int("outbound_replies_count", replyCount).
		Int("any_ai_queued", boolToInt(anyAIQueued)).
		Int64("server_time_ms", resp.ServerTime).
		Interface("outbound_replies", redactOutboundReplies(resp.OutboundReplies)).
		Msg("[Bridge HTTP] ingest 响应（每条结果 + AI 回复摘要）")

	c.JSON(http.StatusOK, resp)
}

// ───────────────────────── 桥接下发三通道（2026-08-06 架构重构） ─────────────────────────
//
// 通道A·上报:  POST /api/bridge/ingest            （HandleHTTPIngest，即时返回）
// 通道B·状态:  POST /api/bridge/outbox/ack         （AckBridgeOutbox）
// 通道C·下发:  GET  /api/bridge/outbox             （GetBridgeOutbox）
//
// 设计：4 个渠道把聊天内容推到上报队列（ingest），服务端按会话去重入库；
// AI 回复落 message_hub(status=pending) 作为下发队列；扩展独立轮询 outbox 拉取并转发网页，
// 成功后通过 ack 通道确认 delivered。详见 docs/bridge/REDESIGN-2026-08-06.md。

// BridgeOutboxMessage 下发队列中的一条待发消息（别名 channelgw.OutboxMessage，HTTP/WS 共用序列化）。
type BridgeOutboxMessage = channelgw.OutboxMessage

// BridgeOutboxAckRequest 状态上报请求体（通道B）。
type BridgeOutboxAckRequest struct {
	MsgIDs []string `json:"msg_ids"`
	Status string   `json:"status"`
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
	// 2026-08-14 修复：account_id 不再兜底为 "default"
	//   与 ingest 端点保持一致：account_id 必填，缺则 400 拒绝。
	//   防止扩展 bug（漏发 account_id）让所有"账号未知"的下发拉取都聚到 "default" 这个
	//   错误 bucket，下发数据归属混乱。
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "account_id required (extension must capture account from DOM before polling outbox)",
		})
		return
	}
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "channel required"})
		return
	}
	// 渠道白名单（与 ingest 端点一致，单源化到 channelgw.Default，防止任意 channel 探测）
	if !IsBridgeChannel(channel) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "unsupported bridge channel"})
		return
	}
	ctx := c.Request.Context()
	// 2026-08-14 用户诉求：下发请求完整打 log（query 不截断）。
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
	// limit 由前端 outboxBatchSize 控制（默认 50，封顶 200），实现三通道"要求1：前端参数可配置"
	if q := c.Query("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			if n > 200 {
				n = 200
			}
			// 服务端权威认领：pending→inflight（原子），根除同一条被两轮轮询重复拉取→重复转发。
			hubs, err := h.ingress.ClaimPendingOutbound(ctx, channel, accountID, n)
			if err != nil {
				logger.Ctx(ctx).Error().Err(err).Str("module", "bridge").Str("channel", channel).Msg("[Bridge] ListPendingOutboundLimit failed")
				c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "query outbox failed"})
				return
			}
			// 节点5 下行出库：对拉取的每个 pending 出站记一条 downlink_fetch 节点（一条出站只拉取一次）
			tracing.RecordDownlinkFetchBatch(ctx, channel, accountID, hubs)
			// 2026-08-14 用户诉求：下发响应完整打 log（content 不截断）。
			logger.Ctx(ctx).Info().
				Str("module", "bridge").
				Str("event", "http_outbox_response").
				Str("full_url", c.Request.URL.String()).
				Str("channel", channel).
				Str("account_id", accountID).
				Int("limit", n).
				Int("messages_count", len(hubs)).
				Interface("messages", _bridgeOutboxHubsForLog(hubs)).
				Msg("[Bridge HTTP] outbox 响应（完整待发消息列表，含 content）")
			writeOutboxJSON(c, hubs)
			return
		}
	}
	// 服务端权威认领：pending→inflight（原子），根除重复转发。
	hubs, err := h.ingress.ClaimPendingOutbound(ctx, channel, accountID, 50)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("module", "bridge").Str("channel", channel).Msg("[Bridge] ListPendingOutbound failed")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "query outbox failed"})
		return
	}
	// 节点5 下行出库：对拉取的每个 pending 出站记一条 downlink_fetch 节点
	tracing.RecordDownlinkFetchBatch(ctx, channel, accountID, hubs)
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
			Extra:          hub.Extra, // 2026-08-14：补 Extra 透传，否则主动私信路由失效
		})
	}
	// 2026-08-14 用户诉求：下发响应完整打 log（content 不截断）。
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_response").
		Str("full_url", c.Request.URL.String()).
		Str("channel", channel).
		Str("account_id", accountID).
		Int("limit", 50).
		Int("messages_count", len(hubs)).
		Interface("messages", _bridgeOutboxHubsForLog(hubs)).
		Msg("[Bridge HTTP] outbox 响应（完整待发消息列表，含 content）")
	c.JSON(http.StatusOK, gin.H{"status": "ok", "messages": msgs})
}

// BridgeOutboxAckResponse 状态确认响应（通道B），含 per-msg-id 详细状态（P3-D 2026-08-15）。
//
// 协议契约（与前端 downlink.js 配套）：
//   - items[].status = "acked"        本次成功翻转 pending→delivered
//   - items[].status = "duplicate"    此前已为 delivered，本地重试队列可清空
//   - items[].status = "not_found"    本 (channel, account_id) 下不存在，停止重发
type BridgeOutboxAckResponse struct {
	Status         string                  `json:"status"`
	Acked          int                     `json:"acked"`            // 本次翻转行数
	DuplicateCount int                     `json:"duplicate_count"`  // 已 delivered 幂等跳过
	NotFoundCount  int                     `json:"not_found_count"`  // 不存在
	Items          []service.AckOutboundItem `json:"items,omitempty"` // 每条 msg_id 详细结果
}

// AckBridgeOutbox 桥接下发状态确认（通道B·状态上报）。
// 扩展把消息成功转发到网页后，批量上报 msg_ids，服务端标记为 delivered。
//
// POST /api/bridge/outbox/ack  body: {"msg_ids":[...],"status":"delivered"}
//
// 2026-08-15 P3-D：响应包含 per-msg-id 详细状态（acked/duplicate/not_found），
// 前端 downlink.js 据此精确清理本地重试队列，避免重复发送。
func (h *BridgeIngestHandler) AckBridgeOutbox(c *gin.Context) {
	channel := c.Query("channel")
	accountID := c.Query("account_id")
	// 2026-08-14 修复：account_id 不再兜底为 "default"
	//   与 ingest/outbox 端点保持一致——三个端点共用同一份"必填参数"语义。
	//   防止扩展 bug（漏发 account_id）让所有"账号未知"的 ack 都写入 "default" 桶，
	//   实际消息可能根本不属于 default 账号 → state 错乱 → 后续重发。
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "account_id required (extension must capture account from DOM before acking)",
		})
		return
	}
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "channel required"})
		return
	}
	// 渠道白名单（与 ingest/outbox 端点一致，单源化到 channelgw.Default，防止任意 channel 探测）
	if !IsBridgeChannel(channel) {
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
	// 2026-08-14 用户诉求：ack 请求完整打 log（query + body 不截断）。
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
	// 2026-08-15 P3-D：调用详细版 ack，返回每条 msg_id 的处理状态
	ackResult, err := h.ingress.AckOutboundDeliveredDetailed(ctx, channel, accountID, req.MsgIDs)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("module", "bridge").Str("channel", channel).Msg("[Bridge] AckOutboundDeliveredDetailed failed")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "ack failed"})
		return
	}
	// 2026-08-14 用户诉求：ack 响应完整打 log（acked 影响行数 + 请求中传入的 msg_ids 列表不截断）。
	logger.Ctx(ctx).Info().
		Str("module", "bridge").
		Str("event", "http_outbox_ack_response").
		Str("full_url", c.Request.URL.String()).
		Str("channel", channel).
		Str("account_id", accountID).
		Int("acked_affected_rows", ackResult.AffectedCount).
		Int("duplicate_count", ackResult.DuplicateCount).
		Int("not_found_count", ackResult.NotFoundCount).
		Int("request_msg_ids_count", len(req.MsgIDs)).
		Interface("request_msg_ids", req.MsgIDs).
		Interface("ack_items", ackResult.Items).
		Msg("[Bridge HTTP] outbox ack 响应（delivered 影响行数 + 详细 ack 结果）")
	c.JSON(http.StatusOK, BridgeOutboxAckResponse{
		Status:         "ok",
		Acked:          ackResult.AffectedCount,
		DuplicateCount: ackResult.DuplicateCount,
		NotFoundCount:  ackResult.NotFoundCount,
		Items:          ackResult.Items,
	})
}

// callHandleIngressBatch 走 mock 优先，否则真实 ingress 的 HandleIngressBatch
func (h *BridgeIngestHandler) callHandleIngressBatch(ctx context.Context, events []*model.MessageEvent) (*service.InboxIngressBatchResult, error) {
	if h.mockHandle != nil {
		// mock 场景：退化为逐条 mockHandle，构造 batch 结果
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
	// 渠道归一化（2026-08-13 修复）：上游可能上报 xhs / xhs_web 等历史简写，
	// 必须在转 Event 前统一为全名（xiaohongshu），否则 SessionID/ConversationID 主键
	// 会以 xhs 为前缀，与 message_hub.platform=归一化值分裂，导致统一收件箱关联错位。
	m.Channel = NormalizeBridgeChannel(ToBridgeChannel(m.Channel))
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

// redactOutboundReplies 响应日志脱敏：保留每条 reply 的关键字段，content 截断到 200 字符。
func redactOutboundReplies(replies []*UnifiedReply) []map[string]any {
	out := make([]map[string]any, 0, len(replies))
	for _, r := range replies {
		if r == nil {
			continue
		}
		preview := r.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		out = append(out, map[string]any{
			"channel":           r.Channel,
			"account_id":        r.AccountID,
			"conversation_id":   r.ConversationID,
			"content_preview":   preview,
			"content_length":    len(r.Content),
			"reply_to_event_id": r.ReplyToEventID,
			"truncated":         r.Truncated,
		})
	}
	return out
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
