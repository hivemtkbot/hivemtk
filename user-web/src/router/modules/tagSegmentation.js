export default [
  {
    path: 'tagSegmentation/list',
    name: 'TagSegmentationList',
    component: () => import('@/views/tagSegmentation/List.vue'),
    meta: { title: '标签分层', group: 'customer', icon: 'PriceTag', requiresAuth: true }
  }
]
