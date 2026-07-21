export default [
  {
    path: 'aiAgent',
    name: 'AIAgent',
    component: () => import('@/views/aiAgent/List.vue'),
    meta: { title: '智能体', group: 'aiAgent', icon: 'Cpu' }
  },
  {
    path: 'aiAgent/list',
    name: 'AIAgentList',
    component: () => import('@/views/aiAgent/List.vue'),
    meta: { title: '智能体列表', group: 'aiAgent', icon: 'List' }
  },
  {
    path: 'aiAgent/create',
    name: 'AIAgentCreate',
    component: () => import('@/views/aiAgent/Edit.vue'),
    meta: { title: '创建智能体', group: 'aiAgent', icon: 'Plus' }
  },
  {
    path: 'aiAgent/edit/:id',
    name: 'AIAgentEdit',
    component: () => import('@/views/aiAgent/Edit.vue'),
    meta: { title: '编辑智能体', group: 'aiAgent', icon: 'Edit' }
  }
]
