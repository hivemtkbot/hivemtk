// role.js 角色管理路由模块
//
// 阶段 5：角色管理（MENU_PERMISSION_PLAN.md v3.1 §3.2）
// 路径：/system/roles → views/system/RoleList.vue
export default [
  {
    path: 'system/roles',
    name: 'RoleList',
    component: () => import('@/views/system/RoleList.vue'),
    meta: {
      title: 'role.title',
      group: 'system',
      icon: 'UserFilled',
      requiresAuth: true,
      requiresAdmin: true
    }
  }
]
