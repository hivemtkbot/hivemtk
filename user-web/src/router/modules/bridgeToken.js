// 桥接通道凭证管理（v3 BRIDGE_TOKEN_PROTOCOL）
// 原嵌套结构 bug：1. component: Layout 导致 Layout 嵌套 Layout
//                 2. children path: '/token' 绝对路径脱离父路由
// 修复：改为平铺数组结构，与 tiktok.js 等一致
export default [
  {
    path: '/bridge',
    name: 'BridgeRoot',
    redirect: '/bridge/token',
    meta: { title: '渠道接入', icon: 'Link', requiresAdmin: true }
  },
  {
    path: '/bridge/token',
    name: 'BridgeTokenManagement',
    component: () => import('@/views/bridge/TokenManagement.vue'),
    meta: { title: '桥接凭证', requiresAdmin: true }
  },
  // 兼容旧深链 /token（绝对路径，原代码 children.path = '/token' 脱离父路由遗留）
  {
    path: '/token',
    redirect: '/bridge/token'
  }
]
