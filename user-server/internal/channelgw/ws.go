package channelgw

import (
	"context"
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// =============================================================
// WebSocket 传输（渠道网关，2026-08-10 统一收件箱整合）
//
// 与 HTTP 三通道（POST /api/bridge/ingest、GET /api/bridge/outbox、
// POST /api/bridge/outbox/ack）平级的第二种传输呈现，共享：
//   - 同一入站管道 IngressPipeline（InboxIngressService：去重/人工锁/落库/AI 触发）
//   - 同一下发事实源 message_hub(outbound, pending)：
//     HTTP 传输轮询拉取；WS 传输由服务端主动推 outbound_reply 帧。
//
// 帧协议（见 protocol.go Frame）：
//   上行:  register → inbound_message(message/messages) / history / ping / ack(msg_ids)
//   下行:  registered / register_rejected / ack(results) / outbound_reply / pong
//
// 不丢不重保证：
//   - 入站：event_id(msg_id) 幂等去重由管道统一负责（与 HTTP 完全一致）
//   - 出站：ClaimPendingOutbound 原子认领 pending→inflight；客户端 ack 帧回写
//     delivered；断线/未 ack 的 inflight 由既有惰性回收机制重回 pending。
// =============================================================

// WS 传输默认参数。
const (
	// wsRegisterTimeout 首帧 register 握手超时（未注册即断开，防匿名挂连接）
	wsRegisterTimeout = 15 * time.Second
	// wsReadIdleTimeout 读空闲超时（超过该时长无任何上行帧则视为死连接）
	wsReadIdleTimeout = 90 * time.Second
	// wsWriteTimeout 单帧写超时
	wsWriteTimeout = 10 * time.Second
	// wsPushIntervalDefault 出站推帧轮询间隔（与 HTTP 扩展轮询节奏同量级）
	wsPushIntervalDefault = 2 * time.Second
	// wsPushBatchDefault 单轮推帧认领上限
	wsPushBatchDefault = 20
	// wsMaxFrameSize 单帧最大字节数（与 HTTP ingest body 上限同源考虑，取 1MB）
	wsMaxFrameSize = 1 << 20
	// wsPipelineTimeout 单次管道调用超时（入站批处理/出站认领/ack）
	wsPipelineTimeout = 30 * time.Second
)

// WSTransport WebSocket 传输处理器。
type WSTransport struct {
	pipeline     IngressPipeline
	registry     *Registry
	upgrader     websocket.Upgrader
	pushInterval time.Duration
	pushBatch    int
	// OnRegister 可选钩子：register 校验通过后异步调用（如 upsert 桥接账号）。
	// 由路由装配层注入（channelgw 不依赖 bridge 包，避免导入环）。
	OnRegister func(ctx context.Context, channel, accountID string)
}

// NewWSTransport 构造 WS 传输。registry 为 nil 时使用 Default。
func NewWSTransport(pipeline IngressPipeline, registry *Registry) *WSTransport {
	if registry == nil {
		registry = Default
	}
	return &WSTransport{
		pipeline: pipeline,
		registry: registry,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// 私有化部署单租户场景：与 bridge HTTP 端点同鉴权模型（InitGuard +
			// channel+account_id 自证身份），不限制 Origin；公网多租户部署时
			// 应替换为精确 CheckOrigin。
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		pushInterval: wsPushIntervalDefault,
		pushBatch:    wsPushBatchDefault,
	}
}

// HandleWS gin 处理器：GET /api/ws/channel。
// 握手（register）通过后进入双泵循环，连接断开才返回。
func (t *WSTransport) HandleWS(c *gin.Context) {
	conn, err := t.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warnf("[ChannelGW WS] 协议升级失败: %v", err)
		return
	}

	// ── register 握手：首帧必须为 register，且渠道注册表校验通过 ──
	_ = conn.SetReadDeadline(time.Now().Add(wsRegisterTimeout))
	var reg Frame
	if err := conn.ReadJSON(&reg); err != nil {
		logger.Warnf("[ChannelGW WS] 读取 register 帧失败: %v", err)
		_ = conn.Close()
		return
	}
	channel, accountID, reason := t.validateRegister(&reg)
	if reason != "" {
		logger.Warnf("[ChannelGW WS] 注册拒绝: channel=%s account=%s reason=%s", channel, accountID, reason)
		_ = conn.WriteJSON(&Frame{
			V: CurrentProtocolVersion, Type: FrameRegisterReject,
			Channel: channel, AccountID: accountID, Reason: reason,
		})
		_ = conn.Close()
		return
	}
	// 注册成功后恢复正常读空闲超时
	_ = conn.SetReadDeadline(time.Now().Add(wsReadIdleTimeout))

	cn := &wsConn{
		t:         t,
		conn:      conn,
		channel:   channel,
		accountID: accountID,
		done:      make(chan struct{}),
	}
	logger.Infof("[ChannelGW WS] 渠道连接已注册: channel=%s account=%s", channel, accountID)

	// 异步账号 upsert 钩子（不阻塞握手；失败不影响连接）
	if t.OnRegister != nil {
		go func() {
			defer func() { _ = recover() }()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			t.OnRegister(ctx, channel, accountID)
		}()
	}

	// 双泵：读泵处理上行帧；推泵轮询下发队列推帧。handler 阻塞至连接结束。
	go cn.readPump()
	go cn.pushPump()
	<-cn.done
	logger.Infof("[ChannelGW WS] 渠道连接已断开: channel=%s account=%s", channel, accountID)
}

// validateRegister 校验 register 帧，返回归一化的 (channel, accountID, 拒绝原因)。
// channel/account_id 帧级与消息级均可携带，帧级优先。
func (t *WSTransport) validateRegister(f *Frame) (string, string, string) {
	if f == nil || f.Type != FrameRegister {
		return "", "", "first frame must be register"
	}
	channel := f.Channel
	accountID := f.AccountID
	if f.Message != nil {
		channel = firstNonEmpty(channel, f.Message.Channel)
		accountID = firstNonEmpty(accountID, f.Message.AccountID)
	}
	if channel == "" || accountID == "" {
		return channel, accountID, "channel and account_id required"
	}
	if !t.registry.Supports(channel, TransportWebSocket) {
		return channel, accountID, "channel not registered for websocket transport"
	}
	return channel, accountID, ""
}

// wsConn 单条渠道 WS 连接的运行态。
type wsConn struct {
	t         *WSTransport
	conn      *websocket.Conn
	channel   string
	accountID string

	sendMu    sync.Mutex    // 序列化写（读泵回执与推泵并发写保护）
	done      chan struct{} // 连接生命周期结束信号
	closeOnce sync.Once
}

// close 幂等关闭：广播 done + 关闭底层连接。
func (c *wsConn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// send 线程安全写帧；写失败即关闭连接并返回 false。
func (c *wsConn) send(f *Frame) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	if err := c.conn.WriteJSON(f); err != nil {
		logger.Warnf("[ChannelGW WS] 写帧失败 channel=%s account=%s type=%s: %v",
			c.channel, c.accountID, f.Type, err)
		c.close()
		return false
	}
	return true
}

// readPump 上行帧循环：ping/ack/inbound_message/history。读错误即结束连接。
func (c *wsConn) readPump() {
	defer c.close()
	c.conn.SetReadLimit(wsMaxFrameSize)
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadIdleTimeout))
		var f Frame
		if err := c.conn.ReadJSON(&f); err != nil {
			return
		}
		c.handleFrame(&f)
	}
}

// handleFrame 分派单条上行帧。
func (c *wsConn) handleFrame(f *Frame) {
	switch f.Type {
	case FramePing:
		c.send(&Frame{V: CurrentProtocolVersion, Type: FramePong, Channel: c.channel, AccountID: c.accountID})
	case FrameAck:
		// 客户端确认出站已下发：回写 delivered（幂等）
		if len(f.MsgIDs) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), wsPipelineTimeout)
		defer cancel()
		acked, err := c.t.pipeline.AckOutbound(ctx, c.channel, c.accountID, f.MsgIDs)
		if err != nil {
			logger.Warnf("[ChannelGW WS] ack 回写失败 channel=%s account=%s: %v", c.channel, c.accountID, err)
			return
		}
		logger.Infof("[ChannelGW WS] 出站已确认 delivered: channel=%s account=%s acked=%d", c.channel, c.accountID, acked)
	case FrameInbound:
		c.handleInbound(f)
	case FrameHistory:
		c.handleHistory(f)
	default:
		// 未知帧类型：忽略（前向兼容新客户端扩展帧）
	}
}

// collectFrameMessages 合并帧内 Message 与 Messages（过滤 nil，回填渠道/账号）。
func collectFrameMessages(f *Frame, channel, accountID string) []*IngestMessage {
	msgs := make([]*IngestMessage, 0, len(f.Messages)+1)
	if f.Message != nil {
		msgs = append(msgs, f.Message)
	}
	msgs = append(msgs, f.Messages...)
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if m.Channel == "" {
			m.Channel = channel
		}
		if m.AccountID == "" {
			m.AccountID = accountID
		}
	}
	return msgs
}

// handleInbound 实时入站帧：历史项先落库，实时消息批量进管道，回 ack(results)。
// 语义与 HTTP 传输 HandleHTTPIngest 严格同源（含「实时消息不得作为 history 回填」修复）。
func (c *wsConn) handleInbound(f *Frame) {
	msgs := collectFrameMessages(f, c.channel, c.accountID)
	if len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), wsPipelineTimeout)
	defer cancel()

	events := make([]*model.MessageEvent, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		// 历史上下文回填（仅落库）：跳过与实时消息同 EventID 的项，
		// 防止先落库相同 msg_id 导致实时消息被幂等跳过而永不触发 AI。
		for _, it := range m.History {
			if it == nil {
				continue
			}
			if m.EventID != "" && it.EventID == m.EventID {
				continue
			}
			dir := it.Direction
			if dir == "" {
				dir = "inbound"
			}
			if (m.SenderType == "self" || m.SenderType == "agent") && dir != "outbound" {
				dir = "outbound"
			}
			if err := c.t.pipeline.PersistHistory(ctx, HistoryToEvent(m, it), dir); err != nil {
				logger.Warnf("[ChannelGW WS] history 落库失败 event_id=%s conv=%s: %v",
					it.EventID, m.ConversationID, err)
			}
		}
		events = append(events, m.ToEvent(string(TransportWebSocket)))
	}
	if len(events) == 0 {
		return
	}

	batchResult, err := c.t.pipeline.IngestBatch(ctx, events)
	results := make([]*IngestResult, 0, len(events))
	if err != nil {
		logger.Warnf("[ChannelGW WS] IngestBatch 失败 channel=%s account=%s: %v", c.channel, c.accountID, err)
		for _, ev := range events {
			results = append(results, &IngestResult{EventID: ev.EventID, Accepted: false, Reason: err.Error()})
		}
	} else if batchResult != nil {
		for i, ev := range events {
			r := &IngestResult{EventID: ev.EventID}
			if i < len(batchResult.PerEvent) && batchResult.PerEvent[i] != nil {
				pr := batchResult.PerEvent[i]
				r.Accepted = pr.Accepted
				r.AIHandled = pr.QueuedForAI
				r.Reason = pr.Reason
				r.Duplicate = IsDuplicateReason(pr.Reason)
			}
			results = append(results, r)
		}
	}
	c.send(&Frame{
		V: CurrentProtocolVersion, Type: FrameAck,
		Channel: c.channel, AccountID: c.accountID,
		TraceID: f.TraceID, Results: results,
	})
}

// handleHistory 历史/回填帧：仅落库，不触发 AI。
func (c *wsConn) handleHistory(f *Frame) {
	msgs := collectFrameMessages(f, c.channel, c.accountID)
	if len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), wsPipelineTimeout)
	defer cancel()
	for _, m := range msgs {
		if m == nil {
			continue
		}
		dir := m.Direction
		if dir == "" {
			dir = "inbound"
		}
		if err := c.t.pipeline.PersistHistory(ctx, m.ToEvent(string(TransportWebSocket)), dir); err != nil {
			logger.Warnf("[ChannelGW WS] history 帧落库失败 event_id=%s conv=%s: %v",
				m.EventID, m.ConversationID, err)
		}
	}
}

// pushPump 出站推帧泵：周期性认领下发队列（pending→inflight）并推送 outbound_reply 帧。
// 客户端经 ack 帧回写 delivered；断线未 ack 的行由既有惰性回收重回 pending。
func (c *wsConn) pushPump() {
	ticker := time.NewTicker(c.t.pushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.pushOnce()
		}
	}
}

// pushOnce 单轮推帧：认领 → 逐条推帧（写失败即停止，剩余由回收机制重生）。
func (c *wsConn) pushOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), wsPipelineTimeout)
	hubs, err := c.t.pipeline.ClaimOutbound(ctx, c.channel, c.accountID, c.t.pushBatch)
	cancel()
	if err != nil {
		logger.Warnf("[ChannelGW WS] 认领下发队列失败 channel=%s account=%s: %v", c.channel, c.accountID, err)
		return
	}
	if len(hubs) == 0 {
		return
	}
	// 全链路监控：下行出库节点（与 HTTP outbox 拉取同源可观测）
	tracing.RecordDownlinkFetchBatch(context.Background(), c.channel, c.accountID, hubs)
	for _, hub := range hubs {
		f := &Frame{
			V: CurrentProtocolVersion, Type: FrameOutboundReply,
			Channel: c.channel, AccountID: c.accountID,
			MsgID: hub.MsgID,
			Reply: &OutboundReply{
				Channel:        c.channel,
				AccountID:      c.accountID,
				ConversationID: hub.ConversationID,
				Content:        hub.Content,
				MsgType:        hub.MsgType,
				MediaURL:       hub.MediaURL,
			},
		}
		if !c.send(f) {
			return // 连接已断：剩余 inflight 等待惰性回收
		}
	}
}
