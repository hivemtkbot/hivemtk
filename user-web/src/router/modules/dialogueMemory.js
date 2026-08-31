export default [
  {
    path: '/dialogueMemory/list',
    name: 'DialogueMemoryList',
    component: () => import('@/views/dialogueMemory/List.vue'),
    meta: { title: '对话记忆', group: 'aiAgent', icon: 'ChatDotRound', requiresAuth: true }
  }
]
