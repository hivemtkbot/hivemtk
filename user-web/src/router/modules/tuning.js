export default [{
  path: '/confidence/panel',
  name: 'ConfidencePanel',
  component: () => import('@/views/confidence/Panel.vue'),
  meta: { title: '置信度运营', group: 'aiAgent', icon: 'TrendCharts', requiresAuth: true }
}, {
  path: '/humanize/panel',
  name: 'HumanizePanel',
  component: () => import('@/views/humanize/Panel.vue'),
  meta: { title: '拟人度评估', group: 'aiAgent', icon: 'UserFilled', requiresAuth: true }
}, {
  path: '/feedbackLoop/panel',
  name: 'FeedbackLoopPanel',
  component: () => import('@/views/feedbackLoop/Panel.vue'),
  meta: { title: '反馈学习闭环', group: 'aiAgent', icon: 'Connection', requiresAuth: true }
}];
