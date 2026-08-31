export default [
  {
    path: '/customerEvent/list',
    name: 'CustomerEventList',
    component: () => import('@/views/customerEvent/List.vue'),
    meta: { title: '客户事件', group: 'customer', icon: 'Bell', requiresAuth: true }
  }
]
