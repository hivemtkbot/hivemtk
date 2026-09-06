package tooluse

import "hivemtk-user/internal/aiagent/agent/portcontract"

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
