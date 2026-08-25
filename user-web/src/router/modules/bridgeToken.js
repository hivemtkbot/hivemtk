// 桥接通道凭证管理（v3 BRIDGE_TOKEN_PROTOCOL）
export default {
  path: '/bridge',
  name: 'bridge',
  meta: { title: '渠道接入', icon: 'Link', requiresAdmin: true },
  component: () => import('@/layout/Layout.vue'),
  children: [
    {
      path: 'token',
      name: 'BridgeTokenManagement',
      component: () => import('@/views/bridge/TokenManagement.vue'),
      meta: { title: '桥接凭证', requiresAdmin: true }
    }
  ]
}
