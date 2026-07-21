export default [
  {
    path: 'batchOperation/list',
    name: 'BatchOperationList',
    component: () => import('@/views/batchOperation/List.vue'),
    meta: { title: '批量操作', group: 'reach', icon: 'Operation', requiresAuth: true }
  }
]
