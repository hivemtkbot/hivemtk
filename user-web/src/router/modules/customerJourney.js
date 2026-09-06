export default [{
  path: '/customerJourney/dashboard',
  name: 'CustomerJourneyDashboard',
  component: () => import('@/views/customerJourney/Dashboard.vue'),
  meta: { title: '客户旅程大屏', group: 'analytics', icon: 'TrendCharts', requiresAuth: true }
}];
