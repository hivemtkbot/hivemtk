/**
 * 统一枚举常量索引
 *
 * 所有 user-web 业务枚举的单一入口。
 * 业务视图统一从本目录 import，禁止再各自维护零散的 label/type map。
 *
 * 类别：
 * - channel.js     渠道（wecom/douyin/...）
 * - source.js      系统级来源（auto/manual/llm/...）
 * - enabled.js     启用/禁用（1/0/active/disabled）
 * - role.js        角色（admin/agent/user/ai/...）
 * - msgType.js     消息类型（text/image/link/...）
 * - aiAgentType.js AI 智能体类型（sales/customer_service/hybrid）
 * - assetType.js   资产类型（agent_persona/sales_script/...）
 * - riskLevel.js   风险等级（1/2/3/4）
 * - priority.js    优先级（urgent/high/medium/low）
 * - direction.js   方向（in/out/inbound/outbound）
 * - authType.js    认证类型（bearer/api_key/hmac/basic）
 * - rating.js      反馈评分（1/0/-1）
 * - intentType.js  对话意图（purchase/price_inquiry/...）
 * - trend.js       趋势方向（up/down/flat）
 * - status.js      业务级 status 集（任务/同步/备份/实验/群组/内容/审核/提示词/黑名单/会话/阶段/对话/嵌入/坐席/通过失败/分群类型）
 */

export * as channel from './channel'
export * as source from './source'
export * as enabled from './enabled'
export * as role from './role'
export * as msgType from './msgType'
export * as aiAgentType from './aiAgentType'
export * as assetType from './assetType'
export * as riskLevel from './riskLevel'
export * as priority from './priority'
export * as direction from './direction'
export * as authType from './authType'
export * as rating from './rating'
export * as intentType from './intentType'
export * as trend from './trend'
export * as status from './status'

export { default as channelDefault } from './channel'
export { default as sourceDefault } from './source'
export { default as enabledDefault } from './enabled'
export { default as roleDefault } from './role'
export { default as msgTypeDefault } from './msgType'
export { default as aiAgentTypeDefault } from './aiAgentType'
export { default as assetTypeDefault } from './assetType'
export { default as riskLevelDefault } from './riskLevel'
export { default as priorityDefault } from './priority'
export { default as directionDefault } from './direction'
export { default as authTypeDefault } from './authType'
export { default as ratingDefault } from './rating'
export { default as intentTypeDefault } from './intentType'
export { default as trendDefault } from './trend'
export { default as statusDefault } from './status'
