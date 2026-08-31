export default [
  {
    path: '/wecomAccount/list',
    name: 'WecomAccountList',
    component: () => import('@/views/wecomAccount/List.vue'),
    meta: { title: '企微账号管理', group: 'workspace', icon: 'Connection' }
  },
  {
    path: '/wecomAccount/data',
    name: 'WecomAccountData',
    component: () => import('@/views/wecomAccount/Data.vue'),
    meta: { title: '企微数据看板', group: 'workspace', icon: 'DataLine' }
  }
]
