export default [
  {
    path: 'order/list',
    name: 'OrderList',
    component: () => import('@/views/order/List.vue'),
    meta: { title: '订单管理', group: 'business', icon: 'List', requiresAuth: true }
  }
]
