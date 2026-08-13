export default [
  {
    path: '/douyinCard',
    name: 'DouyinCard',
    component: () => import('@/views/douyinCard/List.vue'),
    meta: { title: '抖音卡片', group: 'community', icon: 'VideoPlay' }
  },
  {
    path: '/douyin/stats',
    name: 'DouyinStats',
    component: () => import('@/views/douyinCard/Stats.vue'),
    meta: { title: '抖音卡片统计', group: 'community', icon: 'DataAnalysis' }
  },
  {
    path: '/douyin-card-stats/:id',
    name: 'DouyinCardStats',
    component: () => import('@/views/douyinCard/CardStats.vue'),
    meta: { title: '抖音卡片详情统计', group: 'community', icon: 'DataAnalysis' }
  }
]
