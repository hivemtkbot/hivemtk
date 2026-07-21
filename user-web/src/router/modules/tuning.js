// 置信度/拟人度/反馈学习 管理面板路由
// 依据：docs/核心链路优化.md 第十五/十六/十七章
export default [
  {
    path: 'confidence/panel',
    name: 'ConfidencePanel',
    component: () => import('@/views/confidence/Panel.vue'),
    meta: { title: '置信度运营', group: 'aiAgent', icon: 'TrendCharts', requiresAuth: true }
  },
  {
    path: 'humanize/panel',
    name: 'HumanizePanel',
    component: () => import('@/views/humanize/Panel.vue'),
    meta: { title: '拟人度评估', group: 'aiAgent', icon: 'UserFilled', requiresAuth: true }
  },
  {
    path: 'feedbackLoop/panel',
    name: 'FeedbackLoopPanel',
    component: () => import('@/views/feedbackLoop/Panel.vue'),
    meta: { title: '反馈学习闭环', group: 'aiAgent', icon: 'Connection', requiresAuth: true }
  }
]
