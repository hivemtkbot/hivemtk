// 阶段5：知识库创建+上传 → 智能体测试对话（执行测试）
import { chromium } from 'playwright'
import { mkdirSync, writeFileSync } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const shots = 'test-results/ux-audit'
mkdirSync(shots, { recursive: true })
const log = (...a) => console.log('[j5]', ...a)

const browser = await chromium.launch()
const ctx = await browser.newContext({ locale: 'zh-CN' })
const page = await ctx.newPage()
page.errors = []
page.on('pageerror', (e) => page.errors.push(String(e)))
const msgs = async () => page.$$eval('.el-message', els => els.map(e => e.textContent.trim()))
const api400 = []
page.on('response', r => { if (r.status() >= 400) api400.push(r.status() + ' ' + r.url()) })
const dismissNotif = async () => {
  const c = page.locator('.message-notification button:has-text("关闭")').first()
  if (await c.count()) { await c.click(); await page.waitForTimeout(400) }
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

// ===== 知识库 =====
await page.evaluate(() => { location.hash = '/knowledgeBase' })
await page.waitForTimeout(2000)
await dismissNotif()
await page.screenshot({ path: `${shots}/30-kb-list.png` })
const btns = await page.$$eval('button', els => els.map(e => e.textContent.trim()).filter(t => t && t.length < 20))
log('kb buttons:', JSON.stringify([...new Set(btns)].slice(0, 20)))
const empties = await page.$$eval('.el-empty', els => els.map(e => e.textContent.trim().slice(0, 60)))
log('kb empty:', JSON.stringify(empties))

// 点新建知识库
const createKb = page.locator('button:has-text("新建"), button:has-text("创建")').first()
if (await createKb.count()) {
  await createKb.click()
  await page.waitForTimeout(1200)
  await page.screenshot({ path: `${shots}/31-kb-create.png` })
  // 填名称
  const dlg = page.locator('.el-dialog:visible, .el-drawer:visible').first()
  const codeInput = dlg.locator('.el-form-item', { hasText: 'KB编码' }).locator('input').first()
  const nameInput = dlg.locator('.el-form-item', { hasText: '名称' }).locator('input').first()
  if (await nameInput.count()) {
    if (await codeInput.count()) await codeInput.fill('ux-audit-kb-' + Date.now() % 10000)
    await nameInput.fill('UX走查知识库' + Date.now() % 10000)
    const confirm = dlg.locator('button:has-text("确定"), button:has-text("创建"), button:has-text("保存")').last()
    await confirm.click()
    await page.waitForTimeout(2000)
    log('kb create msgs:', JSON.stringify(await msgs()))
  }
  await page.screenshot({ path: `${shots}/32-kb-after-create.png` })
} else {
  log('NO create KB button found')
}

// 找上传入口
const uploadBtn = page.locator('button:has-text("上传")').first()
log('upload btn:', await uploadBtn.count())
if (await uploadBtn.count()) {
  // 准备测试文档
  const doc = join(tmpdir(), 'ux_audit_kb.txt')
  writeFileSync(doc, 'UX走查测试知识：本产品提供7天免费试用，试用期满后可按月订阅。客服工作时间9:00-18:00。')
  await uploadBtn.click()
  await page.waitForTimeout(1200)
  await page.screenshot({ path: `${shots}/33-kb-upload.png` })
  const fileInputs = page.locator('input[type="file"]')
  log('file inputs:', await fileInputs.count())
  if (await fileInputs.count()) {
    await fileInputs.first().setInputFiles(doc)
    await page.waitForTimeout(1500)
    // 确认上传
    const upDlg = page.locator('.el-dialog:visible, .el-drawer:visible').last()
    const upBtn = upDlg.locator('button:has-text("上传"), button:has-text("确定"), button:has-text("导入")').last()
    if (await upBtn.count() && await upBtn.isEnabled()) { await upBtn.click(); await page.waitForTimeout(4000) }
    log('upload msgs:', JSON.stringify(await msgs()))
  }
  await page.screenshot({ path: `${shots}/34-kb-after-upload.png` })
}
log('api>=400:', JSON.stringify(api400.slice(0, 12)))
log('errors:', JSON.stringify(page.errors.slice(0, 8)))
await ctx.close(); await browser.close()
