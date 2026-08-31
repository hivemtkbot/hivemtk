// 多语言（I18n）监控看板路由
// 多语言方案：翻译服务运行态监控
export default [
  {
    path: '/i18n/dashboard',
    name: 'I18nDashboard',
    component: () => import('@/views/i18n/Dashboard.vue'),
    meta: { title: '多语言监控', group: 'i18n', icon: 'DataLine', requiresAuth: true }
  }
]
