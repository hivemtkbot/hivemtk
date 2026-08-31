// 销冠画像独立 UI - G9
// 依赖: @/api/persona.js (listStaffs / getPersonaReport)
export default [
  {
    path: '/persona/list',
    name: 'PersonaList',
    component: () => import('@/views/persona/List.vue'),
    meta: { title: '销冠画像', group: 'sales', icon: 'UserFilled', requiresAuth: true }
  }
]
