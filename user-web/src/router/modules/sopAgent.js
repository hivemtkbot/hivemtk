export default [
  {
    path: 'sopAgent/list',
    name: 'SopAgentList',
    component: () => import('@/views/sopAgent/List.vue'),
    meta: { title: '销冠 SOP 智能体', group: 'aiAgent', icon: 'Connection', requiresAuth: true }
  }
]
