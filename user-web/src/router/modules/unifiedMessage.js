export default [
  {
    path: 'unifiedMessage/list',
    name: 'UnifiedMessageList',
    component: () => import('@/views/unifiedMessage/List.vue'),
    meta: { title: '统一消息', group: 'customer', icon: 'Message' }
  }
]
