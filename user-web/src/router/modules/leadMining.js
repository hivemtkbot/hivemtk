// 线索发掘：发掘设置 + 线索库（与 clue.js 约定一致，作为父级布局路由的 children）
export default [
  {
    path: '/lead-mining/index',
    name: 'LeadMining',
    component: () => import('@/views/leadMining/Index.vue'),
    meta: { title: '线索发掘', group: '线索发掘', icon: 'Magic' }
  }
]
