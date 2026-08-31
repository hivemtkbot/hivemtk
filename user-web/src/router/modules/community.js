export default [
  {
    path: '/community/list',
    name: 'CommunityList',
    component: () => import('@/views/community/List.vue'),
    meta: { title: '社群管理', group: 'community', icon: 'UserFilled' }
  }
]
