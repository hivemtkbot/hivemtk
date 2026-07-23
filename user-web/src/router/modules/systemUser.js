// systemUser.js 系统用户（人员管理）路由模块
//
// 阶段 4：人员管理全栈
// 路由：/system/users → UserList.vue
// 鉴权：requiresAuth（必须登录）+ requiresAdmin（仅超管可见，由 router/index.js beforeEach 校验）
//
// 命名规范：与 system.js / operationLog.js / role.js / permission.js 等保持一致
export default [
  {
    path: 'system/users',
    name: 'SystemUserList',
    component: () => import('@/views/system/UserList.vue'),
    meta: {
      title: 'systemUser.title',
      group: 'system',
      icon: 'UserFilled',
      requiresAuth: true,
      requiresAdmin: true
    }
  }
]
