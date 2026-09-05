export default [{
  path: '/system/roles',
  name: 'RoleList',
  component: () => import('@/views/system/RoleList.vue'),
  meta: {
    title: 'role.title',
    group: 'system',
    icon: 'UserFilled',
    requiresAuth: true,
    requiresAdmin: true
  }
}];
