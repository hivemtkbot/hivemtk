export default [
  {
    path: 'dashboardScreen/list',
    name: 'DashboardScreenList',
    component: () => import('@/views/dashboardScreen/List.vue'),
    meta: { title: '数据大屏', group: 'dataAnalysis', icon: 'DataBoard', requiresAuth: true }
  }
]
