export default [
  {
    path: '/backup/list',
    name: 'BackupList',
    component: () => import('@/views/backup/List.vue'),
    meta: { title: '备份恢复', group: 'system', icon: 'FolderOpened', requiresAuth: true, requiresAdmin: true }
  },
  {
    path: '/backup/enhanced',
    name: 'BackupEnhanced',
    component: () => import('@/views/backup/Enhanced.vue'),
    meta: { title: '备份管理', group: 'system', icon: 'Box', requiresAuth: true, requiresAdmin: true }
  }
]
