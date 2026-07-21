export default [
  {
    path: 'conversionFunnel/list',
    name: 'ConversionFunnelList',
    component: () => import('@/views/conversionFunnel/List.vue'),
    meta: { title: '转化漏斗', group: 'analytics', icon: 'Filter', requiresAuth: true }
  }
]
