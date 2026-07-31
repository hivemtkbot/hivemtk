// 2026-07-31 P1-A: FAQ 知识库管理路由
// 路径前缀: /faq
export default [
  {
    path: 'faq',
    name: 'FAQ',
    component: () => import('@/views/faq/Index.vue'),
    meta: { title: 'FAQ 知识库', group: 'aiAgent', icon: 'Notebook' }
  },
  {
    path: 'faq/list',
    name: 'FAQList',
    component: () => import('@/views/faq/List.vue'),
    meta: { title: 'FAQ 列表', group: 'aiAgent', icon: 'List' }
  },
  {
    path: 'faq/editor',
    name: 'FAQEditor',
    component: () => import('@/views/faq/Editor.vue'),
    meta: { title: 'FAQ 编辑', group: 'aiAgent', icon: 'Edit', hideInMenu: true }
  },
  {
    path: 'faq/editor/:id',
    name: 'FAQEditorEdit',
    component: () => import('@/views/faq/Editor.vue'),
    meta: { title: 'FAQ 编辑', group: 'aiAgent', icon: 'Edit', hideInMenu: true }
  }
]
