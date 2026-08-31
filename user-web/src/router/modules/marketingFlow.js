export default [
  {
    path: '/marketingFlow/list',
    name: 'MarketingFlowList',
    component: () => import('@/views/marketingFlow/List.vue'),
    meta: { title: '营销自动化', group: 'reach', icon: 'SetUp', requiresAuth: true }
  }
]
