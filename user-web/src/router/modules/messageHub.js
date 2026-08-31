export default [
  {
    path: '/messageHub/list',
    name: 'MessageHubList',
    component: () => import('@/views/messageHub/List.vue'),
    meta: { title: '消息中台', group: 'workspace', icon: 'MessageBox' }
  }
]
