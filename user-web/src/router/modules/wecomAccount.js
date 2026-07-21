export default [
  {
    path: 'wecomAccount/list',
    name: 'WecomAccountList',
    component: () => import('@/views/wecomAccount/List.vue'),
    meta: { title: '企微账号管理', group: 'workspace', icon: 'Connection' }
  }
]
