const { createRouter, createMemoryHistory } = require('vue-router')
console.log('loaded vue-router ok')
const Layout = { render: () => null }
const Page = { render: () => null }
const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', name: 'Layout', component: Layout, children: [
      { path: 'telegram', name: 'Telegram', component: Page },
    ]},
  ],
})
router.addRoute('Layout', { path: '/douyinCard', name: 'DouyinCard', component: Page })
router.addRoute('Layout', { path: '/douyin-card-stats/:id', name: 'DouyinCardStats', component: Page })
const tests = ['/telegram', '/douyinCard', '/douyin-card-stats/123']
for (const t of tests) {
  try {
    const r = router.resolve(t)
    console.log(t, '=> matched:', r.matched.map(m => m.name).join(' > ') || '(NONE)')
  } catch (e) {
    console.log(t, '=> ERROR', e.message)
  }
}
