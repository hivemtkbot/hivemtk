/**
 * 统一通用枚举：业务级 status
 *
 * 业务 status 多种多样（备份/同步/任务/实验/群组/活动/短链/旅程/卡片/AI 步骤/对话/反馈...），
 * 但底层语义大多是"任务生命周期"。本模块提供 3 套通用 status 集 + 共享辅助函数：
 *
 * 1. TASK_STATUS     通用任务生命周期（pending/running/paused/completed/failed/cancelled）
 * 2. SYNC_STATUS     同步专用（pending/running/success/failed/partial）
 * 3. BACKUP_STATUS   备份专用（pending/running/success/failed/expired）
 * 4. EXPERIMENT_STATUS 实验（draft/running/paused/completed/archived）
 * 5. GROUP_STATUS    群组（active/disabled）
 * 6. CONTENT_STATUS  内容发布（draft/pending/reviewing/published/archived/rejected）
 * 7. AUDIT_STATUS    审核（pending/approved/rejected/reviewing）
 * 8. APPROVAL_STATUS 审批（pending/approved/rejected/cancelled）
 * 9. JOB_STATUS      作业（pending/running/paused/completed/failed）
 * 10. BLACKLIST_STATUS 黑名单（active/expired/removed）
 * 11. DEAL_STATUS     商机（new/contacting/qualified/proposal/won/lost）
 * 12. TICKET_STATUS   工单（open/pending/resolved/closed）
 * 13. PROMPT_STATUS   提示词（draft/reviewing/approved/rejected/archived）
 *
 * 业务视图禁止再各自维护 (pending/running/...) label map，统一使用本文件。
 */

export const TASK_STATUS = Object.freeze([
  { value: 'pending',   label: '等待中',     tagType: 'info',    sort: 1, group: 'task' },
  { value: 'running',   label: '执行中',     tagType: 'warning', sort: 2, group: 'task' },
  { value: 'paused',    label: '已暂停',     tagType: 'info',    sort: 3, group: 'task' },
  { value: 'completed', label: '已完成',     tagType: 'success', sort: 4, group: 'task' },
  { value: 'success',   label: '成功',       tagType: 'success', sort: 4, group: 'task' },
  { value: 'failed',    label: '失败',       tagType: 'danger',  sort: 5, group: 'task' },
  { value: 'cancelled', label: '已取消',     tagType: 'info',    sort: 6, group: 'task' }
])

export const SYNC_STATUS = Object.freeze([
  { value: 'pending', label: '等待中',     tagType: 'info',    sort: 1, group: 'sync' },
  { value: 'running', label: '同步中',     tagType: 'warning', sort: 2, group: 'sync' },
  { value: 'success', label: '成功',       tagType: 'success', sort: 3, group: 'sync' },
  { value: 'failed',  label: '失败',       tagType: 'danger',  sort: 4, group: 'sync' },
  { value: 'partial', label: '部分成功',   tagType: 'warning', sort: 5, group: 'sync' }
])

export const BACKUP_STATUS = Object.freeze([
  { value: 'pending',   label: '等待中',     tagType: 'info',    sort: 1, group: 'backup' },
  { value: 'running',   label: '备份中',     tagType: 'warning', sort: 2, group: 'backup' },
  { value: 'success',   label: '成功',       tagType: 'success', sort: 3, group: 'backup' },
  { value: 'failed',    label: '失败',       tagType: 'danger',  sort: 4, group: 'backup' },
  { value: 'expired',   label: '已过期',     tagType: 'info',    sort: 5, group: 'backup' }
])

export const EXPERIMENT_STATUS = Object.freeze([
  { value: 'draft',     label: '草稿',       tagType: 'info',    sort: 1, group: 'experiment' },
  { value: 'running',   label: '进行中',     tagType: 'success', sort: 2, group: 'experiment' },
  { value: 'paused',    label: '已暂停',     tagType: 'warning', sort: 3, group: 'experiment' },
  { value: 'completed', label: '已完成',     tagType: '',        sort: 4, group: 'experiment' },
  { value: 'archived',  label: '已归档',     tagType: 'info',    sort: 5, group: 'experiment' }
])

export const GROUP_STATUS = Object.freeze([
  { value: 'active',   label: '正常',       tagType: 'success', sort: 1, group: 'group' },
  { value: 'disabled', label: '禁用',       tagType: 'danger',  sort: 2, group: 'group' },
  { value: 'archived', label: '已归档',     tagType: 'info',    sort: 3, group: 'group' }
])

export const CONTENT_STATUS = Object.freeze([
  { value: 'draft',      label: '草稿',       tagType: 'info',    sort: 1, group: 'content' },
  { value: 'pending',    label: '待发布',     tagType: 'warning', sort: 2, group: 'content' },
  { value: 'reviewing',  label: '审核中',     tagType: 'warning', sort: 3, group: 'content' },
  { value: 'published',  label: '已发布',     tagType: 'success', sort: 4, group: 'content' },
  { value: 'rejected',   label: '已驳回',     tagType: 'danger',  sort: 5, group: 'content' },
  { value: 'archived',   label: '已归档',     tagType: 'info',    sort: 6, group: 'content' }
])

export const AUDIT_STATUS = Object.freeze([
  { value: 'pending',   label: '待审核',     tagType: 'warning', sort: 1, group: 'audit' },
  { value: 'reviewing', label: '审核中',     tagType: 'warning', sort: 2, group: 'audit' },
  { value: 'approved',  label: '已通过',     tagType: 'success', sort: 3, group: 'audit' },
  { value: 'rejected',  label: '已驳回',     tagType: 'danger',  sort: 4, group: 'audit' }
])

export const PROMPT_STATUS = Object.freeze([
  { value: 'draft',     label: '草稿',       tagType: 'info',    sort: 1, group: 'prompt' },
  { value: 'reviewing', label: '审核中',     tagType: 'warning', sort: 2, group: 'prompt' },
  { value: 'approved',  label: '已批准',     tagType: 'success', sort: 3, group: 'prompt' },
  { value: 'rejected',  label: '已驳回',     tagType: 'danger',  sort: 4, group: 'prompt' },
  { value: 'archived',  label: '已归档',     tagType: 'info',    sort: 5, group: 'prompt' }
])

export const BLACKLIST_STATUS = Object.freeze([
  { value: 'active',  label: '生效中',     tagType: 'danger',  sort: 1, group: 'blacklist' },
  { value: 'expired', label: '已过期',     tagType: 'info',    sort: 2, group: 'blacklist' },
  { value: 'removed', label: '已解除',     tagType: 'success', sort: 3, group: 'blacklist' }
])

export const SESSION_STATUS = Object.freeze([
  { value: 'open',       label: '进行中',     tagType: 'success', sort: 1, group: 'session' },
  { value: 'pending',    label: '待处理',     tagType: 'warning', sort: 2, group: 'session' },
  { value: 'closed',     label: '已关闭',     tagType: 'info',    sort: 3, group: 'session' },
  { value: 'transferred',label: '已转接',     tagType: 'primary', sort: 4, group: 'session' }
])

export const STAGE_STATUS = Object.freeze([
  { value: 'new',         label: '新阶段',     tagType: 'info',    sort: 1, group: 'stage' },
  { value: 'active',      label: '激活',       tagType: 'success', sort: 2, group: 'stage' },
  { value: 'archived',    label: '已归档',     tagType: 'info',    sort: 3, group: 'stage' }
])

export const CONVERSATION_STATUS = Object.freeze([
  { value: 'open',         label: '未结束',   tagType: 'success', sort: 1, group: 'conversation' },
  { value: 'closed',       label: '已结束',   tagType: 'info',    sort: 2, group: 'conversation' },
  { value: 'pending',      label: '待响应',   tagType: 'warning', sort: 3, group: 'conversation' },
  { value: 'processing',   label: '处理中',   tagType: 'warning', sort: 4, group: 'conversation' }
])

/**
 * 知识库嵌入状态（indexed/processing/failed/pending）
 */
export const EMBED_STATUS = Object.freeze([
  { value: 'indexed',    label: '已索引',   tagType: 'success', sort: 1, group: 'embed' },
  { value: 'processing', label: '处理中',   tagType: 'warning', sort: 2, group: 'embed' },
  { value: 'failed',     label: '失败',     tagType: 'danger',  sort: 3, group: 'embed' },
  { value: 'pending',    label: '待处理',   tagType: 'info',    sort: 4, group: 'embed' }
])

/**
 * 坐席状态（online/busy/offline/away）
 */
export const AGENT_STATUS = Object.freeze([
  { value: 'online',  label: '在线', tagType: 'success', sort: 1, group: 'agent' },
  { value: 'busy',    label: '忙碌', tagType: 'warning', sort: 2, group: 'agent' },
  { value: 'away',    label: '离开', tagType: 'info',    sort: 3, group: 'agent' },
  { value: 'offline', label: '离线', tagType: 'info',    sort: 4, group: 'agent' }
])

/**
 * 通用简单成功/失败状态（用于审计/操作日志等"是/否成功"型状态）
 */
export const PASS_FAIL_STATUS = Object.freeze([
  { value: 'success', label: '成功', tagType: 'success', sort: 1, group: 'passfail' },
  { value: 'failed',  label: '失败', tagType: 'danger',  sort: 2, group: 'passfail' }
])

/**
 * 用户分群类型（active/inactive/auto/static/dynamic/...）
 */
export const SEGMENT_TYPE = Object.freeze([
  { value: 'auto',    label: '自动分群',   tagType: 'primary', sort: 1, group: 'segment' },
  { value: 'manual',  label: '手动分群',   tagType: 'info',    sort: 2, group: 'segment' },
  { value: 'dynamic', label: '动态分群',   tagType: 'success', sort: 3, group: 'segment' },
  { value: 'static',  label: '静态分群',   tagType: '',        sort: 4, group: 'segment' }
])

/**
 * 把任一 STATUS 数组归一化为 { value -> {label, tagType} } 索引。
 * 业务视图应使用本函数生成专用 map 或直接使用顶层 getXxxLabel/Type。
 */
export const buildStatusMaps = (arr) => {
  const labelMap = {}
  const tagTypeMap = {}
  arr.forEach((o) => {
    labelMap[o.value] = o.label
    tagTypeMap[o.value] = o.tagType
  })
  return { labelMap, tagTypeMap }
}

/**
 * 通用业务 status label 查询（优先匹配完整 value，找不到时回退原值）。
 */
export const getStatusLabel = (value, arr) => {
  if (value === undefined || value === null || value === '') return '-'
  const o = arr.find((s) => s.value === value)
  return o ? o.label : String(value)
}

/**
 * 通用业务 status tagType 查询。
 */
export const getStatusTagType = (value, arr) => {
  if (value === undefined || value === null || value === '') return 'info'
  const o = arr.find((s) => s.value === value)
  return o ? o.tagType : 'info'
}

export default {
  TASK_STATUS,
  SYNC_STATUS,
  BACKUP_STATUS,
  EXPERIMENT_STATUS,
  GROUP_STATUS,
  CONTENT_STATUS,
  AUDIT_STATUS,
  PROMPT_STATUS,
  BLACKLIST_STATUS,
  SESSION_STATUS,
  STAGE_STATUS,
  CONVERSATION_STATUS,
  EMBED_STATUS,
  AGENT_STATUS,
  PASS_FAIL_STATUS,
  SEGMENT_TYPE,
  buildStatusMaps,
  getStatusLabel,
  getStatusTagType
}
