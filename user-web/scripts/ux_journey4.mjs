// 阶段4：编辑智能体 → 加载KB树 → 测试对话
import { chromium } from 'playwright'
import { mkdirSync } from 'fs'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j4]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
const msgs = async () => page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))
const api400 = []
page.on('response', r => { if (r.status() >= 400) api400.push(r.status() + ' ' + r.url()) })

await page.goto(BASE + '/#/login')
await page.waitForSelector('.login-box')
await page.locator('.login-box input[type="text"]').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Test@123456')
await Promise.all([
  page.waitForURL((u) => !u.hash.includes('/login'), { timeout: 10000 }),
  page.locator('.login-box button.el-button--primary').click()
])

// 找到刚创建的智能体，点编辑
await page.evaluate(() => { location.hash = '/aiAgent/list' })
await page.waitForTimeout(2000)
const editBtn = page.locator('button:has-text("编辑")').first()
await editBtn.click()
await page.waitForTimeout(2500)
log('edit url:', page.url())

// 加载知识库
const loadKb = page.locator('button:has-text("加载知识库")').first()
if (await loadKb.count()) { await loadKb.click(); await page.waitForTimeout(2500) }
const treeNodes = await page.$$eval('.el-tree-node__label', els => els.map(e => e.textContent.trim()))
log('kb tree:', JSON.stringify(treeNodes.slice(0, 12)))
await page.screenshot({ path: `${shots}/22-edit-kb.png` })

// 关闭可能存在的消息通知弹窗
const notifClose = page.locator('.message-notification button:has-text("关闭"), .message-notification button:has-text("知道了")').first()
if (await notifClose.count()) { await notifClose.click(); await page.waitForTimeout(500); log('notification dismissed') }

// 测试对话
const testBtn = page.locator('button:has-text("测试")').first()
log('test btn:', await testBtn.count())
if (await testBtn.count()) {
  await testBtn.click()
  await page.waitForTimeout(2000)
  await page.screenshot({ path: `${shots}/23-test-panel.png` })
  const chatInput = page.locator('textarea').last()
  const placeholder = await chatInput.getAttribute('placeholder').catch(() => '')
  log('chat placeholder:', placeholder)
  await chatInput.fill('你好')
  const sendBtn = page.locator('button:has-text("发送")').last()
  log('send btn:', await sendBtn.count())
  if (await sendBtn.count()) {
    await sendBtn.click()
    await page.waitForTimeout(10000)
    await page.screenshot({ path: `${shots}/24-chat-reply.png` })
  }
  log('msgs:', JSON.stringify(await msgs()))
}
log('api>=400:', JSON.stringify(api400.slice(0, 10)))
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close(); await browser.close()
