export default [
  {
    path: 'whatsapp',
    name: 'Whatsapp',
    component: () => import('@/views/whatsapp/WhatsappAccount.vue'),
    meta: { title: 'WhatsApp 社群', group: 'community', icon: 'ChatDotRound' }
  },
  {
    path: 'whatsapp/account',
    name: 'WhatsappAccount',
    component: () => import('@/views/whatsapp/WhatsappAccount.vue'),
    meta: { title: '账号管理', group: 'community', icon: 'Cpu' }
  },
  {
    path: 'whatsapp/drafts',
    name: 'WhatsappDrafts',
    component: () => import('@/views/whatsapp/WhatsappDrafts.vue'),
    meta: { title: '草稿箱', group: 'community', icon: 'Document' }
  },
  {
    path: 'whatsapp/jobs',
    name: 'WhatsappJobs',
    component: () => import('@/views/whatsapp/WhatsappJobs.vue'),
    meta: { title: '群发', group: 'community', icon: 'Promotion' }
  },
  {
    path: 'whatsapp/lead-group-selection',
    name: 'LeadGroupSelection',
    component: () => import('@/views/whatsappBot/LeadGroupSelection.vue'),
    meta: { title: '从线索库选择群体', group: 'community', icon: 'User', requiresAuth: true }
  },
  {
    path: 'whatsapp/group-messaging',
    name: 'BulkMessaging',
    component: () => import('@/views/whatsappBot/BulkMessaging.vue'),
    meta: { title: '批量消息发送', group: 'community', icon: 'ChatLineRound', requiresAuth: true }
  }
]