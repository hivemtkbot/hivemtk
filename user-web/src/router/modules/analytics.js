export default [{
  path: '/analytics/cohort-path',
  name: 'CohortPath',
  component: () => import('@/views/analytics/CohortPath.vue'),
  meta: { title: '留存与路径', group: 'analytics', icon: 'TrendCharts', requiresAuth: true }
}];
