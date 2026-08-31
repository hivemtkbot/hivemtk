export default [
  {
    path: '/sms/list',
    name: 'SmsList',
    component: () => import('@/views/sms/List.vue'),
    meta: { title: '短信列表', group: 'reach', icon: 'ChatDotSquare' }
  },
  {
    path: '/sms/drafts',
    name: 'SmsDrafts',
    component: () => import('@/views/sms/Drafts.vue'),
    meta: { title: '短信草稿', group: 'reach', icon: 'Document' }
  },
  {
    path: '/sms/jobs',
    name: 'SmsJobs',
    component: () => import('@/views/sms/Jobs.vue'),
    meta: { title: '短信任务', group: 'reach', icon: 'List' }
  },
  {
    path: '/sms/config',
    name: 'SmsConfig',
    component: () => import('@/views/sms/Config.vue'),
    meta: { title: '短信配置', group: 'reach', icon: 'Setting' }
  }
]