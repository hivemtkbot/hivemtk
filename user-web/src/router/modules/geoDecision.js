// GEO 决策链 L4 报表（v3 度量重构）
export default {
  path: '/geo',
  name: 'geo',
  meta: { title: 'GEO 优化', icon: 'DataAnalysis' },
  component: () => import('@/layout/Layout.vue'),
  children: [
    {
      path: 'decision-report',
      name: 'GeoDecisionReport',
      component: () => import('@/views/geo/DecisionReport.vue'),
      meta: { title: '决策链报表' }
    }
  ]
}
