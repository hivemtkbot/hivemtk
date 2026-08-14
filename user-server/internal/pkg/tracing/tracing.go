package tracing

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/trace"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ToolTraceEvent 工具层一次调用（agent 一轮 / 单次工具）的可观测事件。
// 由 tooluse 包在 observer 中点出，tracing 包负责落库（见 ReportToolCall）。
type ToolTraceEvent struct {
	Kind       string // model.SpanKindAgentTurn | model.SpanKindToolCall
	TraceID    string // 贯穿 Agent Loop 的请求级 trace_id
	AgentID    string
	SessionID  string
	CustomerID string
	CallerID   string
	ToolName   string
	TurnIndex  int // agent 轮次序号（tool_call 继承其所属轮次）
	Input      any
	Output     any
	Error      string
	DurationMs int64
	Status     string // ok | abnormal
}

// ===== 生命周期节点（固定顺序，用于画链路图 + 计算节点间时延） =====
const (
	NodeIngest          = "ingest"
	NodeAIDispatch      = "ai_dispatch"
	NodeOutboundEnqueue = "outbound_enqueue"
	NodeInboxSync       = "inbox_sync"
	NodeDownlinkFetch   = "downlink_fetch"
	NodeDeliveredAck    = "delivered_ack"
	NodeAgentTurn       = model.NodeAgentTurn
	NodeToolCall        = model.NodeToolCall
)

var nodeOrder = map[string]int{
	NodeIngest:          1,
	NodeAIDispatch:      2,
	NodeOutboundEnqueue: 3,
	NodeInboxSync:       4,
	NodeDownlinkFetch:   5,
	NodeDeliveredAck:    6,
}

// NodeOrder 节点在生命周期中的固定位置（未知节点排末尾）。
func NodeOrder(node string) int {
	if o, ok := nodeOrder[node]; ok {
		return o
	}
	return 99
}

var nodeLabels = map[string]string{
	NodeIngest:          "消息上报接入",
	NodeAIDispatch:      "AI 处理编排",
	NodeOutboundEnqueue: "AI 回复出站入队",
	NodeInboxSync:       "收件箱同步",
	NodeDownlinkFetch:   "下行出库拉取",
	NodeDeliveredAck:    "送达确认",
	NodeAgentTurn:       "Agent 一轮推理",
	NodeToolCall:        "工具调用",
}

// NodeLabel 节点中文名（UI 展示）。
func NodeLabel(node string) string {
	if l, ok := nodeLabels[node]; ok {
		return l
	}
	return node
}

// ===== 异步非阻塞 span 投递（核心：追踪绝不阻塞业务主链路） =====
//
// 业务代码调用 RecordNode / Span.End 仅是把 *model.MessageTrace 投递进一个带缓冲的 channel，
// 由独立后台 goroutine 批量落库。当缓冲满时主动丢弃（背压），保证调用方零等待。
var (
	spanCh    chan pendingSpan
	sinkOnce  sync.Once
	dropped   int64
	published int64
	stopCh    chan struct{}
	stopDone  chan struct{}
)

const sinkBuffer = 8192

// pendingSpan 异步 sink 待落库的原始 span：input/output 仍为原始 any，JSON 序列化延迟到
// 后台落库 goroutine 执行，避免在大体积输出（LLM 完整回复、长工具输出）时阻塞业务主链路。
// 业务侧仅做指针传递 + channel 发送（零拷贝、零序列化），最大化降低请求路径延迟。
type pendingSpan struct {
	carrier        *Carrier
	traceID        string
	conversationID string
	accountID      string
	channel        string
	node           string
	nodeOrder      int
	direction      string
	msgID          string
	input          any
	output         any
	durationMs     int64
	expected       string
	status         string
	abnormal       string
	errorStr       string
	parentNode     string
	spanKind       string
	turnIndex      int
	toolName       string
	agentID        string
}

// Init 启动后台批量落库 worker。
// 必须在进程启动时调用一次（通常在 router 初始化 DB 之后）。
// 工具层 observer 由 router 在 Init 后通过 tooluse.ToolTraceSink = tracing.ReportToolCall 接线，
// 从而以观察者模式自动采集 agent 多轮 / 多工具，且不引入 tracing↔tooluse 的循环依赖。
func Init(d *gorm.DB) {
	sinkOnce.Do(func() {
		spanCh = make(chan pendingSpan, sinkBuffer)
		stopCh = make(chan struct{})
		stopDone = make(chan struct{})
		go flushLoop(d)
	})
}

// flushLoop 后台批量写库：每 300ms 或缓冲达到 256 条时落一次。
func flushLoop(d *gorm.DB) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	// 落库批大小与 CreateInBatches 第二参数保持一致（200），避免一次 flush 被拆成 200+56 两次插入。
	batch := make([]*model.MessageTrace, 0, 200)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if d == nil {
			d = db.GetDB()
		}
		if d == nil {
			batch = batch[:0]
			return
		}
		if err := d.CreateInBatches(batch, 200).Error; err != nil {
			logger.Errorf("tracing.flush: batch insert failed: %v", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case span := <-spanCh:
			// 此处才做 JSON 序列化（后台 goroutine，不占业务主链路）。
			batch = append(batch, toModelFromPending(span))
			if len(batch) >= 200 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-stopCh:
			// 优雅退出：排空缓冲区内剩余 span 后落库，避免进程退出丢 trace。
			for {
				select {
				case span := <-spanCh:
					batch = append(batch, toModelFromPending(span))
					if len(batch) >= 200 {
						flush()
					}
				default:
					flush()
					close(stopDone)
					return
				}
			}
		}
	}
}

// Stop 优雅停止后台落库 worker：排空缓冲区并做最后一次批量落库，确保进程退出不丢 trace。
// 幂等：重复调用安全；未 Init（spanCh 为 nil）时直接返回。
func Stop() {
	if spanCh == nil {
		return
	}
	select {
	case <-stopCh:
	default:
		close(stopCh)
	}
	select {
	case <-stopDone:
	case <-time.After(3 * time.Second):
		// 兜底超时：DB 不可用时不在退出流程上无限阻塞。
	}
}

// Publish 非阻塞投递一条 span。缓冲满则丢弃并计数，绝不阻塞调用方。
func Publish(p pendingSpan) {
	if spanCh == nil {
		// 尚未 Init（如单测 / 极早期）：best-effort 同步落库兜底，避免数据丢失。
		if d := db.GetDB(); d != nil {
			_ = d.Create(toModelFromPending(p)).Error
		}
		return
	}
	atomic.AddInt64(&published, 1)
	select {
	case spanCh <- p:
	default:
		atomic.AddInt64(&dropped, 1)
	}
}

// Stats 返回投递统计（运维/健康巡检用）。
// 注意：不可使用命名返回值，否则 &published 会取到局部返回变量（恒为 0），
// 导致 Stats() 永远返回 0 —— 监控面板看到的 trace 投递/丢弃率将失真。
func Stats() (int64, int64) {
	return atomic.LoadInt64(&published), atomic.LoadInt64(&dropped)
}

// ===== 上下文载体（context 传播，减少业务手动透传 trace_id） =====
//
// 一次业务处理（一条消息）在入口构造一个 Carrier，存入 context，沿调用链透传。
// 子 span（工具调用 / 多轮）通过 CarrierFromContext 自动继承会话/渠道维度，无需逐层传参。
type carrierKey struct{}

// Carrier 一次业务处理的归属信息，随 context 透传。
type Carrier struct {
	TraceID        string
	ConversationID string
	AccountID      string
	Channel        string
	MsgID          string
	Direction      string
}

// NewCarrier 构造新载体（自动生成 trace_id）。
func NewCarrier(channel, account, conv string) *Carrier {
	return &Carrier{
		TraceID:        GenerateTraceID(),
		Channel:        channel,
		AccountID:      account,
		ConversationID: conv,
	}
}

// WithCarrier 把载体注入 context。
func WithCarrier(ctx context.Context, c *Carrier) context.Context {
	return context.WithValue(ctx, carrierKey{}, c)
}

// CarrierFromContext 从 context 取出载体；无载体时降级为仅含请求级 trace_id 的空载体。
func CarrierFromContext(ctx context.Context) *Carrier {
	if ctx != nil {
		if c, ok := ctx.Value(carrierKey{}).(*Carrier); ok && c != nil {
			return c
		}
		if tid := trace.TraceIDFromContext(ctx); tid != "" {
			return &Carrier{TraceID: tid}
		}
	}
	return nil
}

// Child 复制会话/渠道维度（保留同一轮 trace_id），用于子 span 挂在同一个 trace 下。
func (c *Carrier) Child() *Carrier {
	if c == nil {
		return &Carrier{TraceID: GenerateTraceID()}
	}
	cp := *c
	return &cp
}

// WithMsgID 返回携带 msg_id 的副本。
func (c *Carrier) WithMsgID(id string) *Carrier {
	cp := c.Child()
	cp.MsgID = id
	return cp
}

// ===== 知识库召回关联（自学习：trace → 涉及的知识库 chunk） =====
//
// RAG 检索时把召回的 chunk ID 累积到 ctx，供 ai_dispatch 埋点记录到 trace，
// 后续自学习模块据此调整这些 chunk 的权重。使用独立 ctx value（指针共享），
// 不依赖 Carrier 指针透传，避免子 span Child 复制导致关联丢失。

type recalledChunksKey struct{}

// InitRecalledChunks 在业务入口初始化召回 chunk 容器（应在调用 AI 编排前）。
func InitRecalledChunks(ctx context.Context) context.Context {
	if _, ok := ctx.Value(recalledChunksKey{}).(*[]string); ok {
		return ctx
	}
	s := make([]string, 0, 8)
	return context.WithValue(ctx, recalledChunksKey{}, &s)
}

// RecordRecalledChunks 累积记录本次召回的 chunk（多次 RAG 检索可叠加）。
func RecordRecalledChunks(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	v, ok := ctx.Value(recalledChunksKey{}).(*[]string)
	if !ok {
		return
	}
	for _, id := range ids {
		if id != "" {
			*v = append(*v, id)
		}
	}
}

// RecalledChunksOf 取出本次业务涉及的所有召回 chunk（未去重，调用方按需去重）。
func RecalledChunksOf(ctx context.Context) []string {
	if v, ok := ctx.Value(recalledChunksKey{}).(*[]string); ok {
		return *v
	}
	return nil
}

// ===== 流式 Span API（业务侧优雅、低侵入） =====
//
// 用法：
//
//	defer tracing.Start(ctx, tracing.NodeAIDispatch).Input(req).Expected("生成回复").End(output, err)
//
// End 第二个返回值即业务逻辑返回的 error；耗时自动统计。整行即一个节点的埋点，无需手动
// 拼 NodeSpan、无需关心落库——全部由异步 sink 处理。
type Span struct {
	ctx       context.Context
	carrier   *Carrier
	node      string
	order     int
	kind      string
	parent    string
	turn      int
	tool      string
	agent     string
	direction string
	msgID     string
	input     any
	output    any
	expected  string
	abnormal  string
	start     time.Time
}

// Start 开启一个生命周期节点 span（自动继承 context 中的载体与耗时起点）。
func Start(ctx context.Context, node string) *Span {
	return &Span{
		ctx:     ctx,
		carrier: CarrierFromContext(ctx),
		node:    node,
		order:   NodeOrder(node),
		kind:    model.SpanKindLifecycle,
		start:   time.Now(),
	}
}

// Input 设置入参（任意可 JSON 序列化对象）。
func (s *Span) Input(v any) *Span { s.input = v; return s }

// Output 设置出参（链式可选，也可在 End 时传入）。
func (s *Span) Output(v any) *Span { s.output = v; return s }

// Expected 设置预期结果描述。
func (s *Span) Expected(text string) *Span { s.expected = text; return s }

// MsgID 设置关联消息 ID。
func (s *Span) MsgID(id string) *Span { s.msgID = id; return s }

// Direction 设置方向（inbound/outbound）。
func (s *Span) Direction(d string) *Span { s.direction = d; return s }

// TraceID 显式覆盖 trace_id（如出站复用入站 trace）。
func (s *Span) TraceID(id string) *Span {
	if s.carrier == nil {
		s.carrier = &Carrier{}
	}
	s.carrier.TraceID = id
	return s
}

// Kind 设置层级种类（lifecycle/agent_turn/tool_call）。
func (s *Span) Kind(k string) *Span { s.kind = k; return s }

// Parent 设置父节点名。
func (s *Span) Parent(p string) *Span { s.parent = p; return s }

// Turn 设置 agent 轮次序号。
func (s *Span) Turn(i int) *Span { s.turn = i; return s }

// Tool 设置工具名。
func (s *Span) Tool(name string) *Span { s.tool = name; return s }

// Agent 设置智能体 ID。
func (s *Span) Agent(id string) *Span { s.agent = id; return s }

// Abnormal 设置该 span 异常时的原因详情（空 abnormal 时若发生错误则回退为错误信息），
// 会被持久化到 message_trace.abnormal 列，在监控 Lifecycle / trace-tree 中展示。
func (s *Span) Abnormal(reason string) *Span { s.abnormal = reason; return s }

// End 结束 span 并异步投递。output/err 一般来自业务逻辑返回值。
func (s *Span) End(output any, err error) {
	dur := time.Since(s.start).Milliseconds()
	status := "ok"
	var errStr string
	if err != nil {
		status = "abnormal"
		errStr = ErrStr(err)
	}
	if s.output == nil {
		s.output = output
	}
	Publish(s.toPending(dur, status, errStr))
}

// toPending 在业务 goroutine 仅做字段拷贝（input/output 仍是原始 any），零 JSON 序列化。
func (s *Span) toPending(dur int64, status, errStr string) pendingSpan {
	c := s.carrier
	if c == nil {
		c = &Carrier{}
	}
	tid := c.TraceID
	if tid == "" && s.ctx != nil {
		tid = trace.TraceIDFromContext(s.ctx)
	}
	kind := s.kind
	if kind == "" {
		kind = model.SpanKindLifecycle
	}
	parent := s.parent
	if parent == "" && kind != model.SpanKindLifecycle {
		parent = NodeAIDispatch
	}
	ab := s.abnormal
	if ab == "" && errStr != "" {
		ab = errStr
	}
	return pendingSpan{
		carrier:        c,
		traceID:        tid,
		conversationID: c.ConversationID,
		accountID:      c.AccountID,
		channel:        c.Channel,
		node:           s.node,
		nodeOrder:      s.order,
		direction:      s.direction,
		msgID:          s.msgID,
		input:          s.input,
		output:         s.output,
		durationMs:     dur,
		expected:       s.expected,
		status:         status,
		abnormal:       ab,
		errorStr:       errStr,
		parentNode:     parent,
		spanKind:       kind,
		turnIndex:      s.turn,
		toolName:       s.tool,
		agentID:        s.agent,
	}
}

// toModelFromPending 在后台落库 goroutine 中将 pendingSpan 转为落库模型（此处才做 JSON 序列化，
// 把 CPU 开销从业务主链路移到异步 sink，降低请求路径延迟）。
func toModelFromPending(p pendingSpan) *model.MessageTrace {
	return &model.MessageTrace{
		TraceID:        p.traceID,
		ConversationID: p.conversationID,
		AccountID:      p.accountID,
		Channel:        p.channel,
		Node:           p.node,
		NodeOrder:      p.nodeOrder,
		Direction:      p.direction,
		MsgID:          p.msgID,
		Input:          toJSON(p.input),
		Output:         toJSON(p.output),
		DurationMs:     p.durationMs,
		Expected:       p.expected,
		Status:         p.status,
		Abnormal:       p.abnormal,
		Error:          p.errorStr,
		SpanKind:       p.spanKind,
		ParentNode:     p.parentNode,
		TurnIndex:      p.turnIndex,
		ToolName:       p.toolName,
		AgentID:        p.agentID,
	}
}

// ===== 兼容旧接口：RecordNode（仍可用，内部走异步 sink） =====
//
// 业务重构后推荐使用流式 Span API；此处保留以兼容既有调用点与单测。
type NodeSpan struct {
	TraceID        string
	ConversationID string
	AccountID      string
	Channel        string
	Node           string
	NodeOrder      int
	Direction      string
	MsgID          string
	Input          any
	Output         any
	DurationMs     int64
	Expected       string
	Status         string
	Abnormal       string
	Error          string
}

// 节点状态常量（兼容既有调用点 tracing.StatusOk / tracing.StatusAbnormal）。
const (
	StatusOk       = "ok"
	StatusAbnormal = "abnormal"
	StatusFailed   = "failed"
	StatusSkipped  = "skipped" // 2026-08-15 P4 二次审核：duplicate/幂等跳过专用
)

// RecordNode 记录一个生命周期节点 span（异步非阻塞）。
func RecordNode(ctx context.Context, span NodeSpan) {
	carrier := CarrierFromContext(ctx)
	if carrier == nil {
		carrier = &Carrier{}
	}
	if span.TraceID == "" {
		span.TraceID = carrier.TraceID
	}
	if span.ConversationID == "" {
		span.ConversationID = carrier.ConversationID
	}
	if span.AccountID == "" {
		span.AccountID = carrier.AccountID
	}
	if span.Channel == "" {
		span.Channel = carrier.Channel
	}
	if span.NodeOrder == 0 {
		span.NodeOrder = NodeOrder(span.Node)
	}
	if span.Status == "" {
		span.Status = StatusOk
	}
	Publish(pendingSpan{
		carrier:        carrier,
		traceID:        span.TraceID,
		conversationID: span.ConversationID,
		accountID:      span.AccountID,
		channel:        span.Channel,
		node:           span.Node,
		nodeOrder:      span.NodeOrder,
		direction:      span.Direction,
		msgID:          span.MsgID,
		input:          span.Input,
		output:         span.Output,
		durationMs:     span.DurationMs,
		expected:       span.Expected,
		status:         span.Status,
		abnormal:       span.Abnormal,
		errorStr:       span.Error,
		spanKind:       model.SpanKindLifecycle,
	})
}

// ===== 工具层 observer：把工具调用事件落为 tool_call / agent_turn 子 span =====

// ReportToolCall 由工具层 observer（tooluse.ToolTraceSink）调用，将一次工具/轮次事件转为层级 span。
// 在 router 启动时接线：tooluse.ToolTraceSink = tracing.ReportToolCall。
func ReportToolCall(ctx context.Context, ev ToolTraceEvent) {
	carrier := CarrierFromContext(ctx)
	if carrier == nil {
		carrier = &Carrier{}
	}
	tid := ev.TraceID
	if tid == "" {
		tid = carrier.TraceID
	}
	if tid == "" && ctx != nil {
		tid = trace.TraceIDFromContext(ctx)
	}
	kind := ev.Kind
	if kind == "" {
		kind = model.SpanKindToolCall
	}
	status := ev.Status
	if status == "" {
		if ev.Error != "" {
			status = "abnormal"
		} else {
			status = "ok"
		}
	}
	Publish(pendingSpan{
		carrier:        carrier,
		traceID:        tid,
		conversationID: carrier.ConversationID,
		accountID:      carrier.AccountID,
		channel:        carrier.Channel,
		node:           kind,
		nodeOrder:      NodeOrder(NodeAIDispatch),
		direction:      "outbound",
		input:          ev.Input,
		output:         ev.Output,
		durationMs:     ev.DurationMs,
		status:         status,
		abnormal:       ev.Error,
		errorStr:       ev.Error,
		parentNode:     NodeAIDispatch,
		spanKind:       kind,
		turnIndex:      ev.TurnIndex,
		toolName:       ev.ToolName,
		agentID:        ev.AgentID,
	})
}

// ===== 出站下行批量采集（按 msg_id 去重，防止轮询膨胀） =====

// RecordDownlinkFetchBatch 记录一次下行出库拉取（按 msg_id 去重：同一 msg 只记一次）。
func RecordDownlinkFetchBatch(ctx context.Context, channel, accountID string, hubs []*model.MessageHub) {
	if len(hubs) == 0 {
		return
	}
	carrier := CarrierFromContext(ctx)
	if carrier == nil {
		carrier = &Carrier{}
	}
	var existing []string
	ids := make([]string, 0, len(hubs))
	for _, h := range hubs {
		ids = append(ids, h.MsgID)
	}
	if d := db.GetDB(); d != nil {
		if err := d.Model(&model.MessageTrace{}).
			Where("node = ? AND msg_id IN ?", NodeDownlinkFetch, ids).
			Pluck("msg_id", &existing).Error; err != nil {
			// dedup 查询失败仅影响去重精度（可能重复记 downlink_fetch span），不影响下行主链路；
			// 但须打印错误以便观测，避免静默吞掉 DB 异常。
			logger.Errorf("[tracing] RecordDownlinkFetchBatch dedup query failed (best-effort): %v", err)
		}
	}
	existSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existSet[e] = struct{}{}
	}
	for _, h := range hubs {
		if _, ok := existSet[h.MsgID]; ok {
			continue
		}
		existSet[h.MsgID] = struct{}{}
		RecordNode(ctx, NodeSpan{
			TraceID:        carrier.TraceID,
			ConversationID: h.ConversationID,
			AccountID:      accountID,
			Channel:        channel,
			Node:           NodeDownlinkFetch,
			NodeOrder:      NodeOrder(NodeDownlinkFetch),
			Direction:      "outbound",
			MsgID:          h.MsgID,
			Expected:       "从桥接出站队列取出待下发消息",
			Status:         "ok",
		})
	}
}

// ===== trace_id 工具 =====

// GenerateTraceID 生成全局唯一 trace_id（带 tr- 前缀，>=20 字符）。
func GenerateTraceID() string {
	return "tr-" + uuid.New().String()
}

// NodeTraceID 由会话 + 账号派生稳定的 inbound trace_id（同一会话多次上报共享）。
func NodeTraceID(conversationID, accountID string) string {
	raw := fmt.Sprintf("%s|%s|inbound", conversationID, accountID)
	sum := sha1Sum(raw)
	return "tr-" + sum
}

// LinkInboundTraceID 入站沿用同会话的 inbound trace_id（派生稳定值），保证同一会话多次上报共享 trace。
func LinkInboundTraceID(ctx context.Context, conversationID string) string {
	id := NodeTraceID(conversationID, "")
	if c := CarrierFromContext(ctx); c != nil {
		c.TraceID = id
	}
	return id
}

// LinkOutboundTraceID 出站复用同会话最近一次 inbound 的 trace_id，关联「客户消息↔AI回复」。
func LinkOutboundTraceID(ctx context.Context, conversationID string) string {
	if conversationID == "" {
		id := GenerateTraceID()
		if c := CarrierFromContext(ctx); c != nil {
			c.TraceID = id
		}
		return id
	}
	if d := db.GetDB(); d != nil {
		var t model.MessageTrace
		err := d.Model(&model.MessageTrace{}).
			Where("conversation_id = ? AND direction = ? AND node = ?", conversationID, "inbound", NodeIngest).
			Order("id DESC").Limit(1).
			Pluck("trace_id", &t.TraceID).Error
		if err == nil && t.TraceID != "" {
			if c := CarrierFromContext(ctx); c != nil {
				c.TraceID = t.TraceID
			}
			return t.TraceID
		}
	}
	id := NodeTraceID(conversationID, "")
	if c := CarrierFromContext(ctx); c != nil {
		c.TraceID = id
	}
	return id
}

// ErrStr 把 error 转为可存储字符串（nil → ""）。
func ErrStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// StartSpan 轻量计时器（兼容旧用法：t := tracing.StartSpan(); ...; t.ElapsedMs()）。
type SpanTimer struct{ start time.Time }

// StartSpan 开始计时。
func StartSpan() *SpanTimer { return &SpanTimer{start: time.Now()} }

// ElapsedMs 返回自 StartSpan 以来的毫秒数。
func (t *SpanTimer) ElapsedMs() int64 { return time.Since(t.start).Milliseconds() }

// toJSON 安全序列化任意值为 JSON 字符串（nil/string/[]byte 直通）。
func toJSON(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// sha1Sum 计算 SHA1 十六进制（用于派生稳定 trace_id）。
func sha1Sum(s string) string {
	h := sha1.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
