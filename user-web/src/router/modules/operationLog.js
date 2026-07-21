export default [
  {
    path: 'operationLog/list',
    name: 'OperationLogList',
    component: () => import('@/views/operationLog/List.vue'),
    meta: { title: '操作日志', group: 'system', icon: 'Tickets', requiresAuth: true }
  }
]
