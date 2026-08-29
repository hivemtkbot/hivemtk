export default [
  {
    path: 'knowledge/management',
    name: 'KnowledgeManagement',
    component: () => import('@/views/KnowledgeWorkspace/KnowledgeManagement.vue'),
    meta: { title: '知识库管理', group: 'knowledge', icon: 'Files', requiresAuth: true }
  },
  {
    path: 'knowledge/batch-import',
    name: 'KnowledgeBatchImport',
    component: () => import('@/views/KnowledgeWorkspace/BatchImport.vue'),
    meta: { title: '批量导入', group: 'knowledge', icon: 'UploadFilled', requiresAuth: true }
  },
  {
    path: 'knowledge/playground',
    name: 'KnowledgePlayground',
    component: () => import('@/views/KnowledgeWorkspace/Playground.vue'),
    meta: { title: '检索 Playground', group: 'knowledge', icon: 'Aim', requiresAuth: true }
  },
  {
    path: 'knowledge/connectors',
    name: 'KnowledgeConnectors',
    component: () => import('@/views/KnowledgeWorkspace/Connectors.vue'),
    meta: { title: '外部连接器', group: 'knowledge', icon: 'Link', requiresAuth: true }
  },
  {
    path: 'knowledge/feedbacks',
    name: 'KnowledgeFeedbacks',
    component: () => import('@/views/KnowledgeWorkspace/FeedbackList.vue'),
    meta: { title: '反馈管理', group: 'knowledge', icon: 'ChatLineRound', requiresAuth: true }
  },
  {
    path: 'knowledge/tokens',
    name: 'KnowledgeTokens',
    component: () => import('@/views/KnowledgeWorkspace/ApiToken.vue'),
    meta: { title: 'API Token', group: 'knowledge', icon: 'Key', requiresAuth: true }
  },
  {
    path: 'knowledge/external',
    name: 'KnowledgeExternal',
    component: () => import('@/views/KnowledgeWorkspace/ExternalImport.vue'),
    meta: { title: '外部系统接入', group: 'knowledge', icon: 'Connection', requiresAuth: true }
  },
  {
    path: 'knowledge/statistics',
    name: 'KnowledgeStatistics',
    component: () => import('@/views/KnowledgeWorkspace/KnowledgeStatistics.vue'),
    meta: { title: '知识库统计', group: 'knowledge', icon: 'DataAnalysis', requiresAuth: true }
  },
  {
    path: 'knowledge/openapi',
    name: 'KnowledgeOpenAPI',
    component: () => import('@/views/KnowledgeWorkspace/OpenAPIIntegration.vue'),
    meta: { title: 'OpenAPI 集成', group: 'knowledge', icon: 'Connection', requiresAuth: true }
  }
]
