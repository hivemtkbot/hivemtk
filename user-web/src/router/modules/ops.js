// 路由：运维总览 + AI 销冠驾驶舱（OPT-UX-01/02）
// 注意：路径拼写为 kebab-case，与 router/index.js 的 pathToModule 映射一致
const Layout = () => import('@/layout/Layout.vue')

export default [
  {
    path: '/ops-overview',
    name: 'OpsOverview',
    component: Layout,
    meta: { title: '运维总览', icon: 'DataAnalysis', requiresAdmin: true },
    children: [
      {
        path: '',
        name: 'OpsOverviewIndex',
        component: () => import('@/views/OpsOverview/Index.vue'),
        meta: { title: '运维总览', keepAlive: true }
      }
    ]
  },
  {
    path: '/sales-cockpit',
    name: 'SalesCockpit',
    component: Layout,
    meta: { title: 'AI 销冠驾驶舱', icon: 'TrendCharts' },
    children: [
      {
        path: '',
        name: 'SalesCockpitIndex',
        component: () => import('@/views/SalesCockpit/Index.vue'),
        meta: { title: 'AI 销冠驾驶舱', keepAlive: true }
      }
    ]
  }
]
