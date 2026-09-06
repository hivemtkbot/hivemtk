// 客户会话 · 会话列表数据与前端筛选(纯客户端过滤)
// 由 views/customerSession/List.vue 原样迁出(零行为变更拆分)
// 同时导出跨组件复用的纯工具函数: mapSession / formatTime
import { computed } from 'vue'

// 纯工具:相对时间格式化(今天显示 HH:MM,否则显示 M/D)
export const formatTime = (time) => {
  if (!time) return ''
  const d = new Date(time)
  const now = new Date()
  if (d.toDateString() === now.toDateString()) {
    return d.toTimeString().substring(0, 5)
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
}

// 纯工具:后端会话字段(大/小写两种风格) → 前端统一模型
export const mapSession = (s) => {
  if (!s) return null
  return {
    id: s.ID ?? s.id,
    sessionId: s.SessionID ?? s.session_id,
    customerId: s.UserID ?? s.user_id,
    customerName: s.UserName ?? s.user_name ?? '访客',
    channel: s.Platform ?? s.platform ?? '',
    status: s.Status ?? s.status ?? 'waiting',
    handlerType: s.HandlerType ?? s.handler_type ?? 'ai',
    lastMessage: s.LastMessage ?? s.last_message ?? '',
    lastTime: s.LastMessageAt ?? s.last_message_at ?? s.CreatedAt ?? s.created_at ?? '',
    createdAt: s.CreatedAt ?? s.created_at,
    unread: s.unread_count ?? s.unread ?? 0,
    tags: s.Tags ?? s.tags ?? []
  };
}

// sessions 为共享状态,由 useSessionList 持有并传入,保证筛选与数据源同源
export function useSessionFilters(sessions) {
  const filterStatus = ref('')

  const SESSION_STATUS_META = {
    pending: { label: '待处理', tagType: 'info' },
    ai_handling: { label: 'AI处理', tagType: 'primary' },
    human_handling: { label: '人工', tagType: 'success' },
    waiting: { label: '等待', tagType: 'warning' },
    resolved: { label: '已解决', tagType: 'success' },
    closed: { label: '已关闭', tagType: 'info' }
  };

  const getSessionStatusLabel = (status) => SESSION_STATUS_META[status]?.label || status
  const getSessionStatusTagType = (status) => SESSION_STATUS_META[status]?.tagType || 'info'

  const ACTIVE_STATUSES = ['pending', 'ai_handling', 'human_handling', 'waiting'];

  const filteredSessions = computed(() => {
    if (!filterStatus.value) return sessions.value
    if (filterStatus.value === '__active__') {
      return sessions.value.filter((s) => ACTIVE_STATUSES.includes(s.status))
    }
    return sessions.value.filter((s) => s.status === filterStatus.value)
  })

  return {
    filterStatus,
    filteredSessions,
    SESSION_STATUS_META,
    getSessionStatusLabel,
    getSessionStatusTagType,
    ACTIVE_STATUSES
  }
}
