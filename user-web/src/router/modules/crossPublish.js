export default [
  {
    path: '/cards/cross-publish',
    name: 'CrossPlatformPublish',
    component: () => import('@/components/cards/CrossPlatformPublisher.vue'),
    meta: {
      title: '跨平台一键发布',
      group: 'community',
      icon: 'Promotion',
      tag: 'OPT-UX-03',
    },
  },
]
