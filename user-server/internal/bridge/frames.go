package bridge

import gw "hivemtk-user/internal/channelgw"

// 协议版本（别名引用 channelgw 权威定义）：
//   - v1: 初始版本（无 v 字段，默认按 v1 处理）
//   - v2: 加 protocol_version 字段、frame 增加 v 字段；保留 v1 向后兼容
//   - 后续破坏性变更必须 bump v+1，并在 channelgw 写清迁移路径
const (
	ProtocolVersionV1      = gw.ProtocolVersionV1
	ProtocolVersionV2      = gw.ProtocolVersionV2
	CurrentProtocolVersion = gw.CurrentProtocolVersion
)

// 帧类型常量（扩展 <-> 服务器 双向）
const (
	FrameRegister      = gw.FrameRegister
	FrameInbound       = gw.FrameInbound
	FrameHistory       = gw.FrameHistory
	FramePong          = gw.FramePong
	FrameAck           = gw.FrameAck
	FramePing          = gw.FramePing
	FrameOutboundReply = gw.FrameOutboundReply
	FrameConfigPush    = gw.FrameConfigPush
)

// HistoryItem 会话级 history 帧中的单轮消息（别名 channelgw.HistoryItem）。
type HistoryItem = gw.HistoryItem

// UnifiedMessage 扩展 -> 服务器 上行统一消息（别名 channelgw.IngestMessage）。
//
// 字段名与 JSON tag 与历史版本完全兼容；新增 ContentHash 字段用于回环去重钩子2。
type UnifiedMessage = gw.IngestMessage

// UnifiedReply 服务器 -> 扩展 下行统一回复（别名 channelgw.OutboundReply）。
type UnifiedReply = gw.OutboundReply

// Frame 通用帧（别名 channelgw.Frame；MessageEventID/ProtocolVersion 方法随类型迁移）。
type Frame = gw.Frame
