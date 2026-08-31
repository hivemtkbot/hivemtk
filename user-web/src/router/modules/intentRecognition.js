export default [
  {
    path: '/intentRecognition/list',
    name: 'IntentRecognitionList',
    component: () => import('@/views/intentRecognition/List.vue'),
    meta: { title: '意图识别', group: 'aiAgent', icon: 'Aim', requiresAuth: true }
  }
]
