export default [
  {
    path: '/scriptTemplate/list',
    name: 'ScriptTemplateList',
    component: () => import('@/views/scriptTemplate/List.vue'),
    meta: { title: '话术库', group: 'aiAgent', icon: 'ChatLineSquare', requiresAuth: true }
  },
  {
    path: '/scriptTemplate/ab-stats',
    name: 'ScriptTemplateAbStats',
    component: () => import('@/views/scriptTemplate/AbStats.vue'),
    meta: { title: '话术AB测试', group: 'aiAgent', icon: 'DataLine', requiresAuth: true }
  }
]
