export default [
  {
    path: '/llmRouting/list',
    name: 'LlmRoutingList',
    component: () => import('@/views/llmRouting/List.vue'),
    meta: { title: 'LLM路由', group: 'aiAgent', icon: 'Cpu', requiresAuth: true }
  },
  {
    path: '/llmRouting/cost',
    name: 'LlmRoutingCostDashboard',
    component: () => import('@/views/llmRouting/CostDashboard.vue'),
    meta: { title: '成本看板', group: 'aiAgent', icon: 'DataLine', requiresAuth: true }
  }
]
