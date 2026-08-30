export default [
  {
    path: 'geo/decision-report',
    name: 'GeoDecisionReport',
    component: () => import('@/views/geo/DecisionReport.vue'),
    meta: { title: '决策链报表', group: 'analytics', icon: 'DataAnalysis', requiresAuth: true }
  },
  {
    path: 'geo-tools/keyword-mining',
    name: 'GeoKeywordMining',
    component: () => import('@/views/geo/KeywordMining.vue'),
    meta: { title: '关键词蒸馏', group: 'analytics', icon: 'Search', requiresAuth: true }
  },
  {
    path: 'geo-tools/content-creation',
    name: 'GeoContentCreation',
    component: () => import('@/views/geo/ContentCreation.vue'),
    meta: { title: '内容创作', group: 'analytics', icon: 'EditPen', requiresAuth: true }
  },
  {
    path: 'geo-tools/content-optimize',
    name: 'GeoContentOptimize',
    component: () => import('@/views/geo/ContentOptimize.vue'),
    meta: { title: '文章优化', group: 'analytics', icon: 'Document', requiresAuth: true }
  },
  {
    path: 'geo-tools/verification',
    name: 'GeoVerification',
    component: () => import('@/views/geo/Verification.vue'),
    meta: { title: '多模型验证', group: 'analytics', icon: 'CircleCheck', requiresAuth: true }
  },
  {
    path: 'geo-tools/reports',
    name: 'GeoReports',
    component: () => import('@/views/geo/Reports.vue'),
    meta: { title: '数据报表', group: 'analytics', icon: 'DataAnalysis', requiresAuth: true }
  },
  {
    path: 'geo-tools/config',
    name: 'GeoConfig',
    component: () => import('@/views/geo/ConfigOptimizer.vue'),
    meta: { title: '配置优化', group: 'analytics', icon: 'Setting', requiresAuth: true }
  }
]
