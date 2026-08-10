package tooluse

// ============================================================================
// 业务 Port 接口（L4 依赖反转）
// ----------------------------------------------------------------------------
// 本文件以 type alias 形式从 `internal/aiagent/agent/portcontract` 重新导出
// Port 接口 + DTO。
//
// 依赖拓扑：
//   service ──→ portcontract ←── tooluse
//
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §七
// ============================================================================

import "hivemtk-user/internal/aiagent/agent/portcontract"

// ----- 类型别名（保持向後兼容，零侵入） -----

// CustomerIdentity 客户身份投影。
type CustomerIdentity = portcontract.CustomerIdentity

// CustomerProfileView 客户 360 视图投影。
type CustomerProfileView = portcontract.CustomerProfileView

// CustomerPort 客户域端口。
type CustomerPort = portcontract.CustomerPort

// CreateSessionInput 创建会话输入。
type CreateSessionInput = portcontract.CreateSessionInput

// SendMessageInput 发送消息输入。
type SendMessageInput = portcontract.SendMessageInput

// SessionPort 会话域端口。
type SessionPort = portcontract.SessionPort

// FollowUpScheduleOptions 跟进任务选项。
type FollowUpScheduleOptions = portcontract.FollowUpScheduleOptions

// FollowUpPort 跟进域端口。
type FollowUpPort = portcontract.FollowUpPort

// JourneyPort 客户旅程端口。
type JourneyPort = portcontract.JourneyPort

// ReachSendInput 触达发送输入。
type ReachSendInput = portcontract.ReachSendInput

// ReachPipelinePort 触达管线端口。
type ReachPipelinePort = portcontract.ReachPipelinePort
