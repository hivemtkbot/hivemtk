export default [
  {
    path: 'backup/list',
    name: 'BackupList',
    component: () => import('@/views/backup/List.vue'),
    meta: { title: '备份恢复', group: 'system', icon: 'FolderOpened', requiresAuth: true }
  }
]
