export default [
  {
    path: 'licenseManagement/list',
    name: 'LicenseManagementList',
    component: () => import('@/views/licenseManagement/List.vue'),
    meta: { title: '授权管理', group: 'system', icon: 'Key', requiresAuth: true }
  }
]
