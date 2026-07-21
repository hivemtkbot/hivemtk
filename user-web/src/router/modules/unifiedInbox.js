export default [
  {
    path: 'unifiedInbox/list',
    name: 'UnifiedInboxList',
    component: () => import('@/views/unifiedInbox/List.vue'),
    meta: { title: '统一收件箱', group: 'workspace', icon: 'Inbox' }
  }
]
