export default [{
  path: '/system/users',
  name: 'SystemUserList',
  component: () => import('@/views/system/UserList.vue'),
  meta: {
    title: 'systemUser.title',
    group: 'system',
    icon: 'UserFilled',
    requiresAuth: true,
    requiresAdmin: true
  }
}];
