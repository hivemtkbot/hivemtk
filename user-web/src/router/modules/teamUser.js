export default [
  {
    path: 'teamUser/list',
    name: 'TeamUserList',
    component: () => import('@/views/teamUser/List.vue'),
    meta: { title: '团队成员', group: 'system', icon: 'UserFilled', requiresAuth: true }
  },
  {
    path: 'teamUser/role',
    name: 'TeamUserRole',
    component: () => import('@/views/teamUser/Role.vue'),
    meta: { title: '角色权限', group: 'system', icon: 'Lock', requiresAuth: true }
  }
]
