export default [
  {
    path: 'llmRouting/list',
    name: 'LlmRoutingList',
    component: () => import('@/views/llmRouting/List.vue'),
    meta: { title: 'LLM路由', group: 'aiAgent', icon: 'Cpu', requiresAuth: true }
  }
]
