package model

// 桥接 ack 协议状态常量（权威定义，L5 共享层）。
//
// 2026-08-15 P0-7 协议常量单源化：
//   此前 "acked"/"failed"/"duplicate"/"not_found"/"not_in_scope"/"delivered" 六个状态
//   字符串散落在 bridge(handler) / service / repository 三层，一旦改动极易漏改导致
//   前后端协议漂移。现将它们收敛到 model 包（L5 共享层），service / repository /
//   bridge 一律引用常量，禁止业务代码再写协议字面量。
//
// 协议契约（与前端 downlink.js / http-ingest.js 的 BRIDGE_PROTOCOL_V2 严格对齐）：
//   - ack 请求终态：delivered | failed（P0-3）
//   - ack 响应 items[].status：acked | failed | duplicate | not_found | not_in_scope
const (
	BridgeAckStatusDelivered = "delivered"
	BridgeAckStatusFailed = "failed"

	BridgeAckStatusAcked = "acked"
	BridgeAckStatusDuplicate = "duplicate"
	BridgeAckStatusNotFound = "not_found"
	BridgeAckStatusNotInScope = "not_in_scope"
)

