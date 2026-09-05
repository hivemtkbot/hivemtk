export default [{
  path: '/system/permissions',
  name: 'PermissionPanel',
  component: () => import('@/views/system/PermissionPanel.vue'),
  meta: {
    title: 'permission.title',
    group: 'system',
    icon: 'Lock',
    requiresAuth: true,
    requiresAdmin: true
  }
}];
