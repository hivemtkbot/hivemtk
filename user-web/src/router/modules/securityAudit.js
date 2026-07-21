export default [
  {
    path: 'securityAudit/list',
    name: 'SecurityAuditList',
    component: () => import('@/views/securityAudit/List.vue'),
    meta: { title: '安全审计', group: 'system', icon: 'Shield', requiresAuth: true }
  }
]
