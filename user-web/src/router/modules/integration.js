export default [
  {
    path: '/integration/list',
    name: 'IntegrationList',
    component: () => import('@/views/integration/List.vue'),
    meta: { title: '第三方对接', group: 'system', icon: 'Connection', requiresAuth: true, requiresAdmin: true }
  }
]
