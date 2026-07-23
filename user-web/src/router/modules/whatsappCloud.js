export default {
  path: '/whatsapp-cloud',
  name: 'WhatsappCloud',
  component: () => import('@/views/whatsappCloud/WhatsappCloudAccount.vue'),
  meta: {
    title: 'WhatsApp Cloud',
    icon: 'ChatDotRound',
    group: 'community'
  }
}
