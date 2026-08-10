package bridge

// 协议类型单源化（2026-08-10 统一收件箱 × 渠道网关整合）：
//
// bridge 包的线路协议类型全部收敛为 channelgw（渠道网关）的别名。
// channelgw.IngestMessage 是唯一线路协议（合并了历史 UnifiedMessage /
// HTTPIngestMessage 两套结构），HTTP 三通道与 WebSocket 传输共用同一协议与
// 入站管道；本文件仅保留别名与常量引用，保证既有调用方（handler/测试/文档）
// 零改动兼容。
//
// 协议版本与帧语义的权威定义见 internal/channelgw/protocol.go。

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
	FrameRegister      = gw.FrameRegister      // 扩展上行：注册 channel+account
	FrameInbound       = gw.FrameInbound       // 扩展上行：实时新私信（触发 AI）
	FrameHistory       = gw.FrameHistory       // 扩展上行：历史/回填消息（仅落库，不触发 AI）
	FramePong          = gw.FramePong          // 扩展上行：保活
	FrameAck           = gw.FrameAck           // 扩展上行：下行确认
	FramePing          = gw.FramePing          // 服务器下行：保活
	FrameOutboundReply = gw.FrameOutboundReply // 服务器下行：AI 回复
	FrameConfigPush    = gw.FrameConfigPush    // 服务器下行：配置推送
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
