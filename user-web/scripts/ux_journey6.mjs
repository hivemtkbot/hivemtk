// 阶段6：收件箱/SOP/触达计划/菜单 走查（zh 环境）
import { chromium } from 'playwright'
import { mkdirSync } from 'fs'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j6]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
const api400 = []
page.on('response', r => { if (r.status() >= 400) api400.push(Math.floor(r.status()/100)*100 + ' ' + r.url()) })
const dismissNotif = async () => {
  const c = page.locator('.message-notification button:has-text("关闭")').first()
  if (await c.count()) { await c.click(); await page.waitForTimeout(300) }
}

await page.goto(BASE + '/#/login')
await page.waitForSelector('.login-box')
await page.locator('.login-box input[type="text"]').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Test@123456')
await Promise.all([
  page.waitForURL((u) => !u.hash.includes('/login'), { timeout: 10000 }),
  page.locator('.login-box button.el-button--primary').click()
])
await dismissNotif()

// 菜单结构审查：展开所有顶级菜单
const menuTop = await page.$$eval('.el-menu--horizontal .el-menu-item, .el-menu--horizontal .el-sub-menu__title', els => els.map(e => e.textContent.trim()))
log('top menu:', JSON.stringify(menuTop))
const sideTop = await page.$$eval('.el-menu:not(.el-menu--horizontal) > .el-sub-menu__title, .el-menu:not(.el-menu--horizontal) > .el-menu-item', els => els.map(e => e.textContent.trim()))
log('side groups:', JSON.stringify(sideTop))

const targets = [
  ['/unifiedInbox/list', '40-inbox'],
  ['/sopTemplate/list', '41-sop'],
  ['/sopTemplate/market', '42-sop-market'],
  ['/reachPipeline/list', '43-reach'],
  ['/marketingFlow/list', '44-flow'],
  ['/sms/list', '45-sms']
]
for (const [p, name] of targets) {
  await page.evaluate((h) => { location.hash = h }, p)
  await page.waitForTimeout(2000)
  await dismissNotif()
  const empties = await page.$$eval('.el-empty', els => els.map(e => e.textContent.trim().slice(0, 50)))
  const nodata = await page.$$eval('.el-table__empty-text', els => els.map(e => e.textContent.trim()))
  log(`${p} | empty=${JSON.stringify(empties)} nodata=${JSON.stringify(nodata)}`)
  await page.screenshot({ path: `${shots}/${name}.png` })
}
log('api>=400:', JSON.stringify([...new Set(api400)].slice(0, 15)))
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close(); await browser.close()
