package websocket

import "sync/atomic"

// ============================================================================
// 消息序号（Sequence Number）
// ----------------------------------------------------------------------------
// 设计动机：
//   - 客户端重连时通过 since_seq 拉取增量消息
//   - 客户端按 seq 单调递增排序，解决多协程发送乱序
//   - 客户端发 ack(seq) 告诉服务端"我收到了"
//
// 分配策略：
//   - 全局原子 uint64 计数器
//   - 从 1 递增，0 保留为"未分配"哨兵
//   - 单调递增（不重用），保证 seq 在客户端生命周期内唯一
// ============================================================================

// globalSeq 全局消息序号计数器（atomic uint64）
var globalSeq uint64

// NextSeq 原子获取下一个消息序号（从 1 开始）
//
// 并发安全：使用 atomic.AddUint64 保证全局唯一递增。
// 重启后归零：客户端连接时记录的 since_seq 大于重启后新分配的 seq
// 视为"超出保留范围"，直接清空 since_seq 走全量补发。
func NextSeq() uint64 {
	return atomic.AddUint64(&globalSeq, 1)
}

// PeekSeq 偷看当前 seq（不递增），用于诊断与测试
func PeekSeq() uint64 {
	return atomic.LoadUint64(&globalSeq)
}
