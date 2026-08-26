// 阶段3：完整填表创建 → 绑KB → 测试对话
import { chromium } from 'playwright'
import { mkdirSync } from 'fs'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j3]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
page.on('console', (m) => { if (m.type() === 'error') page.errors.push(m.text()) })
const msgs = async () => page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))

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

await page.locator('.el-form-item', { hasText: '智能体编码' }).first().locator('input').fill('ux_audit_agent_' + Date.now() % 10000)
await page.locator('.el-form-item', { hasText: '智能体名称' }).first().locator('input').fill('UX走查智能体' + Date.now() % 10000)
// 人设描述
const persona = page.locator('.el-form-item', { hasText: '人设描述' }).first()
await persona.locator('textarea').fill('你是一名友好的客服助手')
const desc = page.locator('.el-form-item', { hasText: /^描述/ }).first()
if (await desc.count()) await desc.locator('textarea').first().fill('UX走查用').catch(() => {})

await page.locator('button:has-text("创建智能体")').last().click()
await page.waitForTimeout(3000)
log('after create url:', page.url())
log('msgs:', JSON.stringify(await msgs()))
await page.screenshot({ path: `${shots}/18-created.png` })

if (/aiAgent\/edit\/\d+/.test(page.url())) {
  const loadKb = page.locator('button:has-text("加载知识库")').first()
  if (await loadKb.count()) { await loadKb.click(); await page.waitForTimeout(2000) }
  const treeNodes = await page.$$eval('.el-tree-node__label', els => els.map(e => e.textContent.trim()))
  log('kb tree:', JSON.stringify(treeNodes.slice(0, 12)))
  await page.screenshot({ path: `${shots}/19-kb.png` })
  // 测试按钮（顶部 Test）
  const testBtn = page.locator('button:has-text("测试")').first()
  if (await testBtn.count()) {
    await testBtn.click()
    await page.waitForTimeout(2000)
    await page.screenshot({ path: `${shots}/20-test.png` })
    // 对话输入
    const chatInput = page.locator('.el-drawer textarea, .el-dialog textarea').first()
    log('chat input count:', await chatInput.count())
    if (await chatInput.count()) {
      await chatInput.fill('你好，介绍一下你自己')
      const send = page.locator('.el-drawer button:has-text("发送"), .el-dialog button:has-text("发送"), .el-drawer .el-button--primary, .el-dialog .el-button--primary').first()
      if (await send.count()) { await send.click(); await page.waitForTimeout(8000) }
      await page.screenshot({ path: `${shots}/21-chat-reply.png` })
    }
  }
}
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close(); await browser.close()
