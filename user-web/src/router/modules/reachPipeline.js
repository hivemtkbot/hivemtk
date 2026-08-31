export default [
  {
    path: '/reachPipeline/list',
    name: 'ReachPipelineList',
    component: () => import('@/views/reachPipeline/List.vue'),
    meta: { title: '触达Pipeline', group: 'reach', icon: 'Promotion' }
  }
]
