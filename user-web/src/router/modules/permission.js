// permission.js 授权管理路由模块
//
// 阶段 6：授权管理（MENU_PERMISSION_PLAN.md v3.1 §3.4）
// 路径：/system/permissions → views/system/PermissionPanel.vue
export default [
  {
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
  }
]
