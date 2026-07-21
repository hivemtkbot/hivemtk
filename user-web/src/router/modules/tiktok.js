export default [
  {
    path: 'tiktok',
    name: 'TikTok',
    redirect: '/tiktok/list',
    meta: { title: 'TikTok', group: 'reach', icon: 'VideoPlay' },
    children: [
      {
        path: 'list',
        name: 'TikTokList',
        component: () => import('@/views/tiktokCard/List.vue'),
        meta: { title: 'TikTok卡片', group: 'reach', icon: 'VideoPlay', requiresAuth: true }
      },
      {
        path: 'stats',
        name: 'TikTokStats',
        component: () => import('@/views/tiktokCard/Stats.vue'),
        meta: { title: 'TikTok统计', group: 'reach', icon: 'DataAnalysis', requiresAuth: true }
      },
      {
        path: 'card-stats/:id',
        name: 'TikTokCardStats',
        component: () => import('@/views/tiktokCard/CardStats.vue'),
        meta: { title: 'TikTok卡片统计', group: 'reach', icon: 'DataAnalysis', requiresAuth: true },
        props: true
      },
      {
        path: 'auto-reply',
        name: 'TikTokAutoReply',
        component: () => import('@/views/tiktokCard/AutoReply.vue'),
        meta: { title: 'TikTok自动回复', group: 'reach', icon: 'ChatDotRound', requiresAuth: true }
      }
    ]
  }
]