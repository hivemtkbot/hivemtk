export default [{
  path: '/feishu',
  name: 'Feishu',
  component: () => import('@/views/feishu/FeishuAccount.vue'),
  meta: { title: '飞书', group: 'community', icon: 'ChatLineRound' }
}, {
  path: '/feishu/account',
  name: 'FeishuAccount',
  component: () => import('@/views/feishu/FeishuAccount.vue'),
  meta: { title: '飞书账号', group: 'community', icon: 'Cpu' }
}];
