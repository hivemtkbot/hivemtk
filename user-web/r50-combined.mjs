import { chromium } from 'playwright'
import { readFileSync, writeFileSync } from 'fs'

// R50 四源综合判定 UI 测试
// 每页记录: 时间窗 + API调用(method,url,status) + 控制台(全级别) + pageerror + 交互列表
// 事后由 Python 关联: 服务端日志窗口 + DB 增量 → 每页四源裁决
const BASE = 'http://localhost:8212'
const routes = readFileSync('/tmp/all_routes.txt', 'utf-8').split('\n').map(s => s.trim()).filter(Boolean)
routes.push('/backup/enhanced','/email/deliverability','/knowledge/rag-eval','/userSegment/rfm-matrix','/analytics/cohort-path','/knowledge/connectors','/system/automation-hub','/help-center')

const browser = await chromium.launch({ headless: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
await ctx.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: BASE })
const page = await ctx.newPage()

let cur = null
const apiCalls = []       // {route, t, method, url, status}
const consoleMsgs = []    // {route, t, level, text}
const interactions = []   // {route, kind, label}
let t0Page = null

page.on('request', (r) => { if (r.url().includes('/api/')) page._req = r })
page.on('response', async (r) => {
  if (!r.url().includes('/api/')) return
  const st = r.status()
  let bodySnip = ''
  // R50 修复: 仅在异常状态读 body（正常响应读体会拖慢登录时序导致全 sweep 掉登录态）
  if (st >= 400 || st >= 500) {
    try { bodySnip = (await r.text()).slice(0, 160).replace(/\s+/g, ' ') } catch { }
  }
  apiCalls.push({ route: cur, t: new Date().toISOString(), method: r.request().method(), url: r.url().replace(BASE, '').split('?')[0], status: st, body: bodySnip })
})
page.on('console', (m) => consoleMsgs.push({ route: cur, t: new Date().toISOString(), level: m.type(), text: m.text().slice(0, 140) }))
page.on('pageerror', (e) => consoleMsgs.push({ route: cur, t: new Date().toISOString(), level: 'pageerror', text: String(e).slice(0, 140) }))

await page.goto(BASE + '/login', { waitUntil: 'networkidle', timeout: 30000 })
await page.fill('input[type="text"]', 'admin')
await page.fill('input[type="password"]', 'Admin@12345678')
await page.click('button:has-text("Log In")')
// R50 修复: 等待登录完成（hash 变化或 5s 兜底）
for (let i = 0; i < 25; i++) {
  const h = await page.evaluate(() => location.hash).catch(() => '')
  if (h && !h.includes('login')) break
  await page.waitForTimeout(300)
}
await page.waitForTimeout(1200)

async function settle() {
  for (let i = 0; i < 4; i++) {
    const cancel = page.locator('.el-message-box__btns button:has-text("取消")').first()
    if (await cancel.count() > 0 && await cancel.isVisible().catch(() => false)) { await cancel.click({ timeout: 700 }).catch(() => {}); await page.waitForTimeout(220); continue }
    const ok = page.locator('.el-message-box__btns button:has-text("确定"), .el-message-box__btns button:has-text("确认")').first()
    if (await ok.count() > 0 && await ok.isVisible().catch(() => false)) { await ok.click({ timeout: 700 }).catch(() => {}); await page.waitForTimeout(320); continue }
    const close = page.locator('.el-dialog__headerbtn:visible, .el-drawer__close-btn:visible').first()
    if (await close.count() > 0) { await close.click({ timeout: 700 }).catch(() => {}); await page.waitForTimeout(220); continue }
    break
  }
  await page.keyboard.press('Escape')
  await page.waitForTimeout(120)
}

const pageWindows = []
let totalInteractions = 0

for (const r of routes) {
  cur = r
  t0Page = new Date().toISOString()
  const before = interactions.length
  try {
    await page.evaluate((h) => { location.hash = h }, r)
    await page.waitForTimeout(1250)

    const els = []
    for (const b of await page.locator('button:visible').all()) {
      const t = (await b.textContent().catch(() => '') || '').trim().slice(0, 24)
      els.push({ kind: 'button', loc: b, label: t || '(icon)' })
    }
    for (const t of await page.locator('.el-tabs__item:visible').all()) els.push({ kind: 'tab', loc: t, label: (await t.textContent().catch(() => '') || '').trim().slice(0, 20) })
    for (const sw of await page.locator('.el-switch:visible').all()) els.push({ kind: 'switch', loc: sw, label: 'switch' })
    for (const pg of (await page.locator('.el-pagination .btn-next:visible').all()).slice(0, 1)) els.push({ kind: 'pagination', loc: pg, label: 'next' })

    for (const el of els) {
      if (/退出|登出|注销/.test(el.label)) continue
      try {
        if (!(await el.loc.isVisible().catch(() => false))) continue
        if (!(await el.loc.isEnabled().catch(() => false))) continue
        await el.loc.click({ timeout: 1400, force: el.kind === 'switch' })
        interactions.push({ route: r, kind: el.kind, label: el.label })
        await page.waitForTimeout(430)
        await settle()
      } catch { }
    }
    totalInteractions = interactions.length
  } catch { }
  pageWindows.push({ route: r, t_start: t0Page, t_end: new Date().toISOString(), interactions: interactions.slice(before).length })
}

console.log(`sweep done: ${routes.length} pages, ${totalInteractions} interactions`)
writeFileSync('/tmp/r50_timeline.json', JSON.stringify({ pageWindows, apiCalls, consoleMsgs, interactions }, null, 1))
await browser.close()
