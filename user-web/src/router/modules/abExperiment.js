export default [
  {
    path: '/abExperiment/list',
    name: 'AbExperimentList',
    component: () => import('@/views/abExperiment/List.vue'),
    meta: { title: 'A/B 实验', group: 'dataAnalysis', icon: 'DataLine', requiresAuth: true }
  }
]
