// UX 旅程走查脚本：真实用户视角逐步走核心旅程，记录卡点
// 用法: node scripts/ux_journey.mjs [stage]
import { chromium } from 'playwright'

const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const stage = process.argv[2] || 'login'
const shots = 'test-results/ux-audit'
import { mkdirSync } from 'fs'
mkdirSync(shots, { recursive: true })

const issues = []
const log = (...a) => console.log('[journey]', ...a)
const issue = (level, page, desc) => { issues.push({ level, page, desc }); log(`ISSUE[${level}] ${page}: ${desc}`) }

async function newPage(browser) {
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  page.errors = []
  page.on('pageerror', (e) => page.errors.push(String(e)))
  page.on('console', (m) => { if (m.type() === 'error') page.errors.push(m.text()) })
  return { ctx, page }
}

async function login(page) {
  await page.goto(BASE + '/#/login')
  await page.waitForSelector('.login-box', { timeout: 15000 })
  await page.locator('.login-box input[type="text"]').first().fill('admin')
  await page.locator('.login-box input[type="password"]').fill('Test@123456')
  await Promise.all([
    page.waitForURL((u) => !u.hash.includes('/login'), { timeout: 10000 }),
    page.locator('.login-box button.el-button--primary').click()
  ])
}

async function snap(page, name) {
  await page.screenshot({ path: `${shots}/${name}.png`, fullPage: false })
  log(`shot -> ${shots}/${name}.png`)
}

async function gotoHash(page, path) {
  await page.evaluate((p) => { location.hash = p }, path)
  await page.waitForTimeout(1500)
}

async function visibleMessages(page) {
  return page.$$eval('.el-message, .el-message__content', els => els.map(e => e.textContent.trim()))
}

const browser = await chromium.launch()

if (stage === 'login') {
  const { ctx, page } = await newPage(browser)
  // 未登录访问首页 → 是否重定向登录
  await page.goto(BASE + '/#/')
  await page.waitForTimeout(2000)
  if (!page.url().includes('login')) issue('中', '/', '未登录访问 / 未跳转登录页')
  await login(page)
  await snap(page, '01-after-login')
  log('landed:', page.url())
  // 登录后落地页内容
  const bodyText = (await page.textContent('body')).slice(0, 0)
  // 空状态检查：找 el-empty
  const empties = await page.$$eval('.el-empty', els => els.map(e => e.textContent.trim().slice(0, 80)))
  if (empties.length) log('empty states on landing:', empties)
  await ctx.close()
}

if (stage === 'agent') {
  const { ctx, page } = await newPage(browser)
  await login(page)
  await gotoHash(page, '/aiAgent/list')
  await snap(page, '10-agent-list')
  const empties = await page.$$eval('.el-empty', els => els.map(e => e.textContent.trim().slice(0, 80)))
  log('agent-list empty:', empties)
  const btns = await page.$$eval('button', els => els.map(e => e.textContent.trim()).filter(t => t && t.length < 20))
  log('buttons:', JSON.stringify(btns.slice(0, 30)))
  // 点击"新建"类按钮
  const createBtn = page.locator('button', { hasText: /新建|创建|新增|New/ }).first()
  if (await createBtn.count()) {
    await createBtn.click()
    await page.waitForTimeout(1500)
    log('after create click url:', page.url())
    await snap(page, '11-agent-create-form')
    // 表单字段
    const inputs = await page.$$eval('.el-form-item__label', els => els.map(e => e.textContent.trim()))
    log('form labels:', JSON.stringify(inputs))
    const submitBtns = await page.$$eval('button', els => els.map(e => e.textContent.trim()).filter(t => /保存|提交|确定|创建/.test(t)))
    log('submit buttons:', JSON.stringify(submitBtns))
    // 空表单直接提交（看校验提示）
    for (const t of ['保存', '确定', '创建']) {
      const b = page.locator(`button:has-text("${t}")`).last()
      if (await b.count() && await b.isEnabled()) {
        await b.click(); await page.waitForTimeout(800)
        log('msgs after empty submit:', JSON.stringify(await visibleMessages(page)))
        break
      }
    }
    await snap(page, '12-agent-empty-submit')
    // 填表提交
    const nameInput = page.locator('.el-form-item input').first()
    await nameInput.fill('UX走查测试智能体' + Date.now() % 10000)
    const ta = page.locator('.el-textarea__inner').first()
    if (await ta.count()) await ta.fill('这是一个用于UX走查的测试智能体描述')
    for (const t of ['保存', '确定', '创建']) {
      const b = page.locator(`button:has-text("${t}")`).last()
      if (await b.count() && await b.isEnabled()) {
        await b.click()
        await page.waitForTimeout(2500)
        log('after submit url:', page.url(), 'msgs:', JSON.stringify(await visibleMessages(page)))
        break
      }
    }
    await snap(page, '13-agent-after-submit')
  } else {
    issue('高', '/aiAgent/list', '找不到新建智能体入口按钮')
  }
  log('page errors so far:', JSON.stringify(page.errors.slice(0, 10)))
  await ctx.close()
}

if (stage === 'pages') {
  // 收件箱 / SOP模板 / 触达计划 静态走查
  const { ctx, page } = await newPage(browser)
  await login(page)
  const targets = [
    ['/unifiedInbox/list', '20-inbox'],
    ['/customerSession/list', '21-session'],
    ['/sopTemplate/list', '22-sop-list'],
    ['/sopTemplate/market', '23-sop-market'],
    ['/reachPipeline/list', '24-reach-list'],
    ['/knowledgeBase', '25-kb']
  ]
  for (const [p, name] of targets) {
    await gotoHash(page, p)
    const empties = await page.$$eval('.el-empty', els => els.map(e => e.textContent.trim().slice(0, 60)))
    const msgs = await visibleMessages(page)
    log(`${p} | empty=${JSON.stringify(empties)} | msgs=${JSON.stringify(msgs)} | errs=${JSON.stringify(page.errors.slice(-3))}`)
    await snap(page, name)
  }
  await ctx.close()
}

await browser.close()
log('done. issues:', JSON.stringify(issues, null, 2))
