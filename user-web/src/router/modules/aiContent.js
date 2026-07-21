export default [
  {
    path: 'aiContent/list',
    name: 'AiContentList',
    component: () => import('@/views/aiContent/List.vue'),
    meta: { title: 'AI 内容创作', group: 'knowledge', icon: 'MagicStick', requiresAuth: true }
  }
]
