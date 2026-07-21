export default [
  {
    path: 'templateMarket/list',
    name: 'TemplateMarketList',
    component: () => import('@/views/templateMarket/List.vue'),
    meta: { title: '模板市场', group: 'knowledge', icon: 'Grid', requiresAuth: true }
  }
]
