// 知识库管理路由
// - 列表页（含 RAG/FAQ/SOP Tab）
// - 后续可扩展创建/编辑（暂用 List 内的弹窗/抽屉，或独立页）
export default [
  {
    path: '/knowledgeBase',
    name: 'KnowledgeBase',
    component: () => import('@/views/knowledgeBase/List.vue'),
    meta: { title: '知识库管理', group: 'knowledge', icon: 'Files' }
  },
  {
    path: '/knowledgeBase/list',
    name: 'KnowledgeBaseList',
    component: () => import('@/views/knowledgeBase/List.vue'),
    meta: { title: '知识库列表', group: 'knowledge', icon: 'List' }
  },
  {
    path: '/knowledgeBase/create',
    name: 'KnowledgeBaseCreate',
    component: () => import('@/views/knowledgeBase/List.vue'),
    meta: { title: '新建知识库', group: 'knowledge', icon: 'Plus' }
  },
  {
    path: '/knowledgeBase/edit/:id',
    name: 'KnowledgeBaseEdit',
    component: () => import('@/views/knowledgeBase/List.vue'),
    meta: { title: '编辑知识库', group: 'knowledge', icon: 'Edit' }
  }
]
