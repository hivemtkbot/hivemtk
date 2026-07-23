export default {
  path: '/dingtalk-app',
  name: 'DingtalkApp',
  component: () => import('@/views/dingtalkApp/DingtalkAppAccount.vue'),
  meta: {
    title: '钉钉应用',
    icon: 'ChatDotRound',
    group: 'community'
  }
}
