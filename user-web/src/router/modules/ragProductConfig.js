export default [
  {
    path: '/system/rag-product-config',
    name: 'RagProductConfigIndex',
    component: () => import('@/views/RagProductConfig/index.vue'),
    meta: { title: 'RAG 主配置', group: 'knowledge', icon: 'ChatLineRound', requiresAuth: true }
  },
  {
    path: '/system/rag-account',
    name: 'RagProductAccountConfig',
    component: () => import('@/views/RagProductConfig/AccountConfig.vue'),
    meta: { title: 'RAG 账号配置', group: 'knowledge', icon: 'Key', requiresAuth: true }
  },
  {
    path: '/system/rag-product',
    name: 'RagProductManagement',
    component: () => import('@/views/RagProductConfig/RagProductManagement.vue'),
    meta: { title: 'RAG 产品管理', group: 'knowledge', icon: 'Goods', requiresAuth: true }
  }
]
