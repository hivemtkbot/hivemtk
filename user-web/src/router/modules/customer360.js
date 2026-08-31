export default [
  {
    path: '/customer360/list',
    name: 'Customer360List',
    component: () => import('@/views/customer360/List.vue'),
    meta: { title: '客户 360', group: 'customer', icon: 'UserFilled', requiresAuth: true }
  }
]
