export default [
  {
    path: '/messageHub/list',
    name: 'MessageHubList',
    component: () => import('@/views/messageHub/List.vue'),
    meta: { title: '消息中台', group: 'workspace', icon: 'MessageBox' }
  },
  {
    path: '/messageHub/dashboard',
    name: 'MessageHubDashboard',
    component: () => import('@/views/messageHub/Dashboard.vue'),
    meta: { title: '消息总览', group: 'workspace', icon: 'DataBoard' }
  }
]
