export default [
  {
    path: '/dashboardScreen/list',
    name: 'DashboardScreenList',
    component: () => import('@/views/dashboardScreen/List.vue'),
    meta: { title: '数据大屏', group: 'dataAnalysis', icon: 'DataBoard', requiresAuth: true }
  },
  {
    path: '/dashboardScreen/builder',
    name: 'DashboardScreenBuilder',
    component: () => import('@/views/dashboardScreen/Builder.vue'),
    meta: { title: '大屏构建', group: 'dataAnalysis', icon: 'SetUp', requiresAuth: true }
  }
]
