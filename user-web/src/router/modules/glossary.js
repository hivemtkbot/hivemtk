export default [{
  path: '/glossary',
  name: 'GlossaryList',
  component: () => import('@/views/glossary/List.vue'),
  meta: { title: '术语表管理', group: 'i18n', icon: 'Collection', requiresAuth: true }
}];
