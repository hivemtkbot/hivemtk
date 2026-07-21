export default [
  {
    path: 'userSegment/list',
    name: 'UserSegmentList',
    component: () => import('@/views/userSegment/List.vue'),
    meta: { title: '用户分层 RFM', group: 'customer', icon: 'PieChart', requiresAuth: true }
  }
]
