// 阶段2：填表创建智能体 → 编辑页绑知识库 → 对话测试入口
import { chromium } from 'playwright'
import { mkdirSync } from 'fs'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j2]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
page.on('console', (m) => { if (m.type() === 'error') page.errors.push(m.text()) })

await page.goto(BASE + '/#/login')
await page.waitForSelector('.login-box')
await page.locator('.login-box input[type="text"]').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Test@123456')
await Promise.all([
  page.waitForURL((u) => !u.hash.includes('/login'), { timeout: 10000 }),
  page.locator('.login-box button.el-button--primary').click()
])

await page.evaluate(() => { location.hash = '/aiAgent/create' })
await page.waitForTimeout(1500)

// 填名称（label=智能体名称 对应的输入）
const nameItem = page.locator('.el-form-item', { hasText: '智能体名称' }).first()
await nameItem.locator('input').fill('UX走查智能体' + (Date.now() % 10000))
const desc = page.locator('.el-textarea__inner').first()
if (await desc.count()) await desc.fill('UX走查用测试智能体')
await page.screenshot({ path: `${shots}/14-filled-form.png` })

// 点击创建智能体
const submit = page.locator('button:has-text("创建智能体")').last()
await submit.click()
await page.waitForTimeout(3000)
log('after create url:', page.url())
const msgs = await page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))
log('msgs:', JSON.stringify(msgs))
await page.screenshot({ path: `${shots}/15-after-create.png` })

if (page.url().includes('/aiAgent/edit/')) {
  const agentId = page.url().match(/edit\/(\d+)/)?.[1]
  log('agent id:', agentId)
  // 找知识库挂载区域，点加载知识库
  const loadKb = page.locator('button:has-text("加载知识库")').first()
  if (await loadKb.count()) {
    await loadKb.click()
    await page.waitForTimeout(2000)
    log('kb loaded, msgs:', JSON.stringify(await page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))))
  }
  await page.screenshot({ path: `${shots}/16-kb-tree.png` })
  // 知识库树内容
  const treeNodes = await page.$$eval('.el-tree-node__label', els => els.map(e => e.textContent.trim()))
  log('kb tree nodes:', JSON.stringify(treeNodes.slice(0, 15)))
  // 测试按钮
  const testBtn = page.locator('button:has-text("测试")').first()
  if (await testBtn.count()) {
    await testBtn.click()
    await page.waitForTimeout(1500)
    await page.screenshot({ path: `${shots}/17-test-dialog.png` })
    log('test dialog visible')
    // 关掉
    const close = page.locator('.el-dialog button:has-text("关"), .el-drawer button').first()
  }
}
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close()
await browser.close()
