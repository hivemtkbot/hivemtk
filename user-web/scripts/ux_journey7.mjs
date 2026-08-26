// 阶段7：收件箱会话详情 + SOP模板新建 + Pipeline新建 交互走查
import { chromium } from 'playwright'
import { mkdirSync } from 'fs'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j7]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
const msgs = async () => page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))
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

// ===== 收件箱：点第一行会话 =====
await page.evaluate(() => { location.hash = '/unifiedInbox/list' })
await page.waitForTimeout(2500)
const row = page.locator('.el-table__body-wrapper tbody tr').first()
if (await row.count()) {
  await row.click()
  await page.waitForTimeout(2000)
  log('after row click url:', page.url())
  await page.screenshot({ path: `${shots}/50-inbox-detail.png` })
  const dlgVisible = await page.locator('.el-drawer:visible, .el-dialog:visible').count()
  log('drawer/dialog:', dlgVisible)
} else {
  log('no rows in inbox table')
}

// ===== SOP 模板：新建 =====
await page.evaluate(() => { location.hash = '/sopTemplate/list' })
await page.waitForTimeout(2000)
await dismissNotif()
await page.screenshot({ path: `${shots}/51-sop-list.png` })
let btns = await page.$$eval('button', els => els.map(e => e.textContent.trim()).filter(t => t && t.length < 20))
log('sop buttons:', JSON.stringify([...new Set(btns)].slice(0, 15)))
const sopCreate = page.locator('button:has-text("新建"), button:has-text("创建")').first()
if (await sopCreate.count()) {
  await sopCreate.click()
  await page.waitForTimeout(1500)
  log('after sop create click url:', page.url())
  await page.screenshot({ path: `${shots}/52-sop-create.png` })
}

// ===== Pipeline 新建 =====
await page.evaluate(() => { location.hash = '/reachPipeline/list' })
await page.waitForTimeout(2000)
const pipeCreate = page.locator('button:has-text("新建 Pipeline")').first()
if (await pipeCreate.count()) {
  await pipeCreate.click()
  await page.waitForTimeout(1500)
  log('after pipeline create click url:', page.url())
  await page.screenshot({ path: `${shots}/53-pipeline-create.png` })
  const inputs = await page.$$eval('.el-form-item__label', els => els.map(e => e.textContent.trim()))
  log('pipeline form labels:', JSON.stringify(inputs.slice(0, 15)))
}
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close(); await browser.close()
