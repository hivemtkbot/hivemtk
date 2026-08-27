package channelgw

import (
	"context"
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)


// WS 传输默认参数。
const (
	wsRegisterTimeout = 15 * time.Second
	wsReadIdleTimeout = 90 * time.Second
	wsWriteTimeout = 10 * time.Second
	wsPushIntervalDefault = 2 * time.Second
	wsPushBatchDefault = 20
	wsMaxFrameSize = 1 << 20
	wsPipelineTimeout = 30 * time.Second
)

// WSTransport WebSocket 传输处理器。
type WSTransport struct {
	pipeline     IngressPipeline
	registry     *Registry
	upgrader     websocket.Upgrader
	pushInterval time.Duration
	pushBatch    int
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
	_ = conn.SetReadDeadline(time.Now().Add(wsReadIdleTimeout))

	cn := &wsConn{
		t:         t,
		conn:      conn,
		channel:   channel,
		accountID: accountID,
		done:      make(chan struct{}),
	}
	logger.Infof("[ChannelGW WS] 渠道连接已注册: channel=%s account=%s", channel, accountID)

	if t.OnRegister != nil {
		// 最高标准审计 P1-3 修复：OnRegister 回调改走 SafeGo（原裸 recover 吞 panic 无日志）
		utils.SafeGo(context.Background(), "channelgw.ws.on_register", func(ctx context.Context) {
			cbCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			t.OnRegister(cbCtx, channel, accountID)
		})
	}

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

	sendMu    sync.Mutex    
	done      chan struct{} 
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
			return 
		}
	}
}

