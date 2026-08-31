export default [
  {
    path: '/customReport/list',
    name: 'CustomReportList',
    component: () => import('@/views/customReport/List.vue'),
    meta: { title: '自定义报表', group: 'dataAnalysis', icon: 'Document', requiresAuth: true }
  }
]
