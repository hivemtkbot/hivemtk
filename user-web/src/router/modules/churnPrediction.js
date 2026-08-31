export default [
  {
    path: '/churnPrediction/list',
    name: 'ChurnPredictionList',
    component: () => import('@/views/churnPrediction/List.vue'),
    meta: { title: '流失预警', group: 'dataAnalysis', icon: 'Warning', requiresAuth: true }
  }
]
