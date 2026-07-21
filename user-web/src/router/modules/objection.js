// 异议处理独立 UI - G9
// 依赖: @/api/objection.js (handleObjection / classifyObjection / listObjectionCategories / recordObjectionUsage)
export default [
  {
    path: 'objection/list',
    name: 'ObjectionList',
    component: () => import('@/views/objection/List.vue'),
    meta: { title: '异议处理', group: 'sales', icon: 'ChatLineRound', requiresAuth: true }
  }
]
