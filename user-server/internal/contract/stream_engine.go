// Package contract 跨层共享接口契约（依赖倒置 / 解耦）
// ============================================================================
// 5 层架构定位: L0 横切层（cross-cutting）— 不属于 controller/service/repository 任何一层
//
// 解决 "controller ↔ service 互相依赖" 循环依赖风险：
//   - controller 需要 service 的能力签名（接口）
//   - service 实现方需不依赖 controller（编译期解耦）
//
// 设计动机（B-005）：
//   1. 接口定义在 contract 包, 双方 (controller 与 service) 都依赖 contract，
//      不直接互相 import，从源头打破潜在循环依赖
//   2. 接口可在 controller 单测中独立 mock（不引入 service 真实依赖）
//   3. service 后续若需要拆分/重组，接口签名由 contract 锁定更稳定
//
// 命名规范：
//   - 全部接口以 Interface 结尾（如 StreamEngineInterface）
//   - 全部接口方法以动词开头（Handle / Match / Record）
//   - 禁止使用 utils.go / common.go（5 层架构强约束）
//
// 后续可扩展接口（如 FeedbackRecorderInterface / KnowledgeRetrieverInterface 等）
// 也应归入此包，遵循相同的命名与目录规则。
package contract

import (
	"context"

	"marketing/internal/dto"
)

// ============================================================================
// 流式销售引擎接口
// ============================================================================

// StreamEngineInterface 流式销售引擎接口（B-005：从 controller 包提到此处）
//
// 实现方: *service.SalesEngine（通过 Go 鸭子类型自动满足）
//
// 设计动机：
//   - Controller 不直接持有 *service.SalesEngine 具体类型（编译期解耦）
//   - 单测可注入 mock 实现（无需启动完整 service 栈）
//   - router 层注入 *service.SalesEngine 时只需传 interface，类型签名更稳定
//
// 演进：
//   - 2026-07-31: 从 internal/controller/chat_ws.go 提取至本包
//   - 后续如新增方法（如 CancelStream / GetSessionSummary），在此处追加
//   - 实现方 *service.SalesEngine 需保持向后兼容
type StreamEngineInterface interface {
	// HandleStream 流式处理销售请求，逐 chunk 回调
	//
	// 返回 false 表示调用方（controller）希望中断流；返回 nil 错误表示正常完成。
	// 任何内部错误会被包装为 dto.StreamChunk{Type: error} 推给客户端。
	HandleStream(ctx context.Context, req *dto.SalesRequest, onChunk func(chunk *dto.StreamChunk) bool) error
}
