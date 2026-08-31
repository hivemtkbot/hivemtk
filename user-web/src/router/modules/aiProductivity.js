export default [
  {
    path: '/aiProductivity/list',
    name: 'AiProductivityList',
    component: () => import('@/views/aiProductivity/List.vue'),
    meta: { title: 'AI产能分析', group: 'analytics', icon: 'DataAnalysis', requiresAuth: true }
  }
]
