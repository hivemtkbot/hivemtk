// 术语表（Glossary）管理路由
// 多语言方案：术语统一与保护管理 UI
export default [
  {
    path: 'glossary',
    name: 'GlossaryList',
    component: () => import('@/views/glossary/List.vue'),
    meta: { title: '术语表管理', group: 'i18n', icon: 'Collection', requiresAuth: true }
  }
]
