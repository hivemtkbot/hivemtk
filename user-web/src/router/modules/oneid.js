export default [{
  path: '/oneid/list',
  name: 'OneIDList',
  component: () => import('@/views/oneid/List.vue'),
  meta: { title: 'OneID 列表', icon: 'List', requiresAuth: true }
}, {
  path: '/oneid/conflicts',
  name: 'OneIDConflicts',
  component: () => import('@/views/oneid/Conflicts.vue'),
  meta: { title: '身份冲突解决', icon: 'Warning', requiresAuth: true }
}, {
  path: '/oneid/merge-rules',
  name: 'OneIDMergeRules',
  component: () => import('@/views/oneid/MergeRuleConfig.vue'),
  meta: { title: '合并规则配置', icon: 'Setting', requiresAuth: true, tag: 'OPT-UX-04' }
}];
