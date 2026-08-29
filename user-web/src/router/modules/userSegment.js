export default [
  {
    path: 'userSegment/list',
    name: 'UserSegmentList',
    component: () => import('@/views/userSegment/List.vue'),
    meta: { title: '用户分层 RFM', group: 'customer', icon: 'PieChart', requiresAuth: true }
  },
  {
    path: 'userSegment/rfm-matrix',
    name: 'RfmMatrix',
    component: () => import('@/views/userSegment/RfmMatrix.vue'),
    meta: { title: 'RFM 矩阵', group: 'analytics', icon: 'Grid', requiresAuth: true }
  }
]
