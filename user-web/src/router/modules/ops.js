const Layout = () => import('@/layout/Layout.vue');

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
