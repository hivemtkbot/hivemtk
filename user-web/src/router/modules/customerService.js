export default [{
  path: '/customerService/agentStatus',
  name: 'AgentStatus',
  component: () => import('@/views/customerService/AgentStatus.vue'),
  meta: { title: '坐席状态', group: 'community', icon: 'Headset', requiresAuth: true }
}, {
  path: '/customerService/quickReply',
  name: 'QuickReply',
  component: () => import('@/views/customerService/QuickReply.vue'),
  meta: { title: '快捷回复', group: 'community', icon: 'ChatLineSquare', requiresAuth: true }
}, {
  path: '/customerService/sessionTag',
  name: 'SessionTag',
  component: () => import('@/views/customerService/SessionTag.vue'),
  meta: { title: '会话标签', group: 'community', icon: 'CollectionTag', requiresAuth: true }
}, {
  path: '/customerService/aiSuggestion',
  name: 'AISuggestion',
  component: () => import('@/views/customerService/AISuggestion.vue'),
  meta: { title: 'AI 建议', group: 'community', icon: 'MagicStick', requiresAuth: true }
}, {
  path: '/customerService/csat',
  name: 'CsatDashboard',
  component: () => import('@/views/customerService/CsatDashboard.vue'),
  meta: { title: 'CSAT 看板', group: 'community', icon: 'Star', requiresAuth: true }
}];
