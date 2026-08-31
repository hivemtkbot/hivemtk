export default [
  {
    path: '/scriptTemplate/list',
    name: 'ScriptTemplateList',
    component: () => import('@/views/scriptTemplate/List.vue'),
    meta: { title: '话术库', group: 'aiAgent', icon: 'ChatLineSquare', requiresAuth: true }
  }
]
