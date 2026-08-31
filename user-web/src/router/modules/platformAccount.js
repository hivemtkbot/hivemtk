export default [
  {
    path: '/platformAccount/list',
    name: 'PlatformAccountList',
    component: () => import('@/views/platformAccount/List.vue'),
    meta: { title: '平台账号', group: 'system', icon: 'Platform', requiresAuth: true, requiresAdmin: true }
  }
]
