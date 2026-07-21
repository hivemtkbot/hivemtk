export default [
  {
    path: 'otaUpgrade/list',
    name: 'OtaUpgradeList',
    component: () => import('@/views/otaUpgrade/List.vue'),
    meta: { title: 'OTA升级', group: 'system', icon: 'Upload', requiresAuth: true }
  }
]
