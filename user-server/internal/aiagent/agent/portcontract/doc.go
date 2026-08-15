// Package portcontract 提供 aiagent 工具层（tooluse）与业务层（service）之间的
// 端口契约（Port interfaces）与数据传输对象（DTO），是六层架构 L4 依赖反转的核心。
//
// ----------------------------------------------------------------------------
// 设计目标
// ----------------------------------------------------------------------------
//
//  1. 切断 import cycle：
//     - tooluse 包只依赖本包（portcontract）+ model/repository
//     - service 包的 adapter 也只依赖本包（portcontract）
//     - tooluse 与 service 不再相互 import，依赖方向变成：
//     service ──→ portcontract ←── tooluse
//
//  2. 单向依赖：业务域 Port 接口的"消费者"在 tooluse 侧（工具实现），
//     "提供者"在 service 侧（业务服务），二者通过本包接口契约解耦。
//
//  3. 工具上下文投影：所有跨层 DTO（CustomerIdentity / CreateSessionInput /
//     FollowUpScheduleOptions / ReachSendInput / CustomerProfileView 等）
//     都在本包定义，避免 tooluse 引用 service 的私有请求结构体。
//
// ----------------------------------------------------------------------------
// 装配时机
// ----------------------------------------------------------------------------
// service 包的 tool_ports_adapter.go 提供所有 Port 接口实现；
// 在 main/cmd/api 启动期通过 setter 注入到 tooluse.Registry 或工具 Deps。
// 工具执行时通过 deps.Customer / deps.Session / deps.Order / deps.FollowUp
// 等端口字段访问业务能力，不再持有具体 service 实例引用。
package portcontract

