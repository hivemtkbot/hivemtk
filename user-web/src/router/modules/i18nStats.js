export default [{
  path: '/i18n/dashboard',
  name: 'I18nDashboard',
  component: () => import('@/views/i18n/Dashboard.vue'),
  meta: { title: '多语言监控', group: 'i18n', icon: 'DataLine', requiresAuth: true }
}];
