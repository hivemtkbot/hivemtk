// 资产包（AssetBundle）路由模块
//
// 文档依据: docs/企业级架构优化/资产包模式.md
//   - §五 开发者 Playground 双栏模式
//   - §六 商户低代码傻瓜模式
//
// 路由列表:
//   /asset-bundle/list            资产包列表（管理）
//   /asset-bundle/playground      开发者 Playground（双栏：messages 编排 + 沙箱内测）
//   /asset-bundle/playground/:aid 复用资产包进入 Playground
//   /asset-bundle/merchant/:aid   商户低代码编辑器（挡板/经营策略/6 维门禁/QA 卡片/乐高卡片）
//   /asset-bundle/merchant-new    商户新建低代码资产包
export default [
  {
    path: 'asset-bundle/list',
    name: 'AssetBundleList',
    component: () => import('@/views/assetBundle/List.vue'),
    meta: { title: '资产包管理', group: 'aiAgent', icon: 'Files', requiresAuth: true }
  },
  {
    path: 'asset-bundle/playground',
    name: 'AssetBundlePlayground',
    component: () => import('@/views/assetBundle/Playground.vue'),
    meta: { title: '开发者 Playground', group: 'aiAgent', icon: 'EditPen', requiresAuth: true }
  },
  {
    path: 'asset-bundle/playground/:aid',
    name: 'AssetBundlePlaygroundEdit',
    component: () => import('@/views/assetBundle/Playground.vue'),
    meta: { title: '开发者 Playground', group: 'aiAgent', requiresAuth: true, hidden: true }
  },
  {
    path: 'asset-bundle/merchant-new',
    name: 'AssetBundleMerchantNew',
    component: () => import('@/views/assetBundle/MerchantEditor.vue'),
    meta: { title: '商户新建话术包', group: 'aiAgent', icon: 'MagicStick', requiresAuth: true }
  },
  {
    path: 'asset-bundle/merchant/:aid',
    name: 'AssetBundleMerchantEdit',
    component: () => import('@/views/assetBundle/MerchantEditor.vue'),
    meta: { title: '商户配置话术包', group: 'aiAgent', requiresAuth: true, hidden: true }
  }
]
