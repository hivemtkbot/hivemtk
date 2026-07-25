// 单页 UI 遍历：登录 → 进入路由 → 逐个点击 .app-main 内交互元素
// 每次交互后重载页面隔离状态；捕获 API 调用 / 控制台错误 / 页面异常 / 错误 UI
// API 500 自动重试一次以过滤间歇性抖动（flaky）；真实 JS 错误才计为 issue
// 用法: node walk.mjs <routePath> [baseUrl] [needsAuth]
import { chromium } from 'playwright'
import { writeFileSync, mkdirSync, existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const REPORTS = join(__dirname, 'reports')
if (!existsSync(REPORTS)) mkdirSync(REPORTS, { recursive: true })

const routePath = process.argv[2]
const base = process.argv[3] || 'http://127.0.0.1:8211'
const needsAuth = process.argv[4] === 'false' ? false : true
const MAX = parseInt(process.env.MAX_INTERACTIONS || '50', 10) // 单页交互上限，控制最坏耗时
if (!routePath) {
  console.error('用法: node walk.mjs <routePath> [baseUrl] [needsAuth]')
  process.exit(2)
}
const safeName = routePath.replace(/^\//, '').replace(/\//g, '_') || 'root'
const reportFile = join(REPORTS, safeName + '.json')

const SEL = [
  'button:not([disabled])',
  'a:not([disabled])',
  '[role="button"]',
  '.el-tabs__item',
  '.el-radio',
  '.el-checkbox',
  '.el-switch',
  '.el-dropdown',
  '.el-select',
  '.el-pagination li',
  'input[type="submit"]',
  '.el-upload',
  '[class*="clickable"]',
].join(',')

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const nowIso = () => new Date().toISOString()
const isRealConsoleErr = (t) =>
  /(500|gateway|Internal Server|TypeError|ReferenceError|Cannot read|is not a function|undefined is not|Maximum call stack)/i.test(t)
// 良性页面异常：SPA 路由切换时对进行中的 axios 请求执行 cancel/abort，
// 产生的未处理 CanceledError/AbortError 属 dev 重载噪声，不计为缺陷。
const isBenignPageErr = (t) =>
  /(cancel|CanceledError|aborted|AbortError|用户取消|user aborted|The operation was aborted|请求已取消)/i.test(t)
const realPageErrors = (arr) => arr.filter((x) => !isBenignPageErr(x))

async function main() {
  const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] })
  const page = await context.newPage()

  const consoleErrors = []
  const consoleWarnings = []
  const pageErrors = []
  const apiCalls = []

  page.on('console', (msg) => {
    const type = msg.type()
    const text = (msg.text() || '').slice(0, 600)
    if (type === 'error') consoleErrors.push(text)
    else if (type === 'warning') consoleWarnings.push(text)
  })
  page.on('pageerror', (err) => {
    pageErrors.push((err && err.stack ? err.stack : String(err)).slice(0, 800))
  })
  page.on('response', async (resp) => {
    const url = resp.url()
    if (!url.includes('/api')) return
    const method = resp.request().method()
    const status = resp.status()
    let sample = ''
    try {
      const ct = resp.headers()['content-type'] || ''
      if (ct.includes('application/json')) sample = (await resp.text()).slice(0, 400)
    } catch {}
    apiCalls.push({ method, url: url.replace(base, ''), status, ok: status < 400, sample })
  })
  page.on('requestfailed', (req) => {
    const url = req.url()
    if (!url.includes('/api')) return
    apiCalls.push({ method: req.method(), url: url.replace(base, ''), status: 0, ok: false, sample: 'requestfailed: ' + (req.failure()?.errorText || '') })
  })
  page.on('dialog', (d) => d.dismiss().catch(() => {}))

  const report = {
    route: routePath, base, needsAuth,
    startedAt: nowIso(), loginOk: null, loaded: false, loadError: null,
    interactions: [],
    summary: { total: 0, withIssue: 0, issues: [] },
  }

  async function navToRoute() {
    await page.goto(base + '/#', { waitUntil: 'domcontentloaded', timeout: 30000 })
    await page.evaluate((p) => { location.hash = '#' + p }, routePath)
    // 等待挂载而非可见性，避免 SPA 重定向期间的可见性竞态误判超时
    await page.waitForSelector('.app-main', { state: 'attached', timeout: 20000 })
    await sleep(1500)
  }

  async function tryInteraction(i) {
    const interaction = {
      index: i, tag: '', text: '', cls: '', clicked: false, clickError: null,
      apiCalls: [], consoleErrors: [], pageErrors: [], uiError: null, dialogOpened: false,
    }
    try {
      await navToRoute()
    } catch (e) {
      interaction.reloadError = String(e).slice(0, 300)
      return interaction
    }
    const apiBefore = apiCalls.length
    const ceBefore = consoleErrors.length
    const peBefore = pageErrors.length
    const main = page.locator('.app-main')
    const el = main.locator(SEL).nth(i)
    try {
      interaction.tag = (await el.evaluate((n) => n.tagName).catch(() => '')) || ''
      interaction.text = (await el.evaluate((n) => (n.innerText || n.getAttribute('title') || n.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim().slice(0, 50)).catch(() => '')) || ''
      interaction.cls = (await el.evaluate((n) => (n.className && n.className.toString ? n.className.toString() : '').split(' ').slice(0, 4).join(' ')).catch(() => '')) || ''
    } catch {}
    try {
      await el.click({ timeout: 5000 })
      interaction.clicked = true
    } catch {
      try {
        await el.click({ timeout: 5000, force: true })
        interaction.clicked = true
      } catch (e2) {
        interaction.clickError = String(e2).slice(0, 200)
      }
    }
    await sleep(1000)
    interaction.apiCalls = apiCalls.slice(apiBefore)
    interaction.consoleErrors = consoleErrors.slice(ceBefore)
    interaction.pageErrors = pageErrors.slice(peBefore)
    try {
      interaction.uiError = await page.evaluate(() => {
        const errs = []
        document.querySelectorAll('.el-message--error .el-message__content').forEach((n) => errs.push(n.innerText.trim()))
        const box = document.querySelector('.el-message-box')
        if (box && box.querySelector('.el-message-box__status--error, .el-icon-error')) {
          const t = box.querySelector('.el-message-box__message')?.innerText?.trim()
          if (t) errs.push('confirm:' + t)
        }
        return errs.length ? errs.join(' | ').slice(0, 200) : null
      })
    } catch {}
    try {
      interaction.dialogOpened = await page.evaluate(() => !!document.querySelector('.el-overlay:not([style*="display: none"])'))
    } catch {}
    await page.keyboard.press('Escape').catch(() => {})
    return interaction
  }

  try {
    if (needsAuth) {
      await page.goto(base + '/#/login', { waitUntil: 'load', timeout: 30000 })
      await page.waitForSelector('.el-input input', { timeout: 10000 })
      const inputs = await page.$$('.el-input input')
      if (inputs.length < 2) throw new Error('登录页输入框数量不足: ' + inputs.length)
      await inputs[0].fill('admin')
      await inputs[1].fill('Seed@123456')
      await page.click('button.el-button--primary', { timeout: 10000 })
      await page.waitForFunction(() => !location.hash.includes('/login'), { timeout: 15000 }).catch(() => {})
      report.loginOk = !!(await page.evaluate(() => localStorage.getItem('token')))
    }

    // 加载页面并计数；若初始 API 出现瞬时限流(500)则重试一次，过滤 dev 环境抖动
    async function loadAndCount() {
      await navToRoute()
      const baseApi = apiCalls.length
      const baseCe = consoleErrors.length
      const basePe = pageErrors.length
      await sleep(800)
      const c = await page.locator('.app-main').locator(SEL).count()
      const newBad = apiCalls.slice(baseApi).some((a) => !a.ok) || pageErrors.slice(basePe).length > 0
      return { count: c, baseApi, baseCe, basePe, newBad }
    }
    let load = await loadAndCount()
    if (load.newBad) {
      await sleep(1500)
      load = await loadAndCount()
    }
    const initialApiCount = load.baseApi
    const initialConsoleErr = load.baseCe
    const initialPageErr = load.basePe
    const rawCount = load.count
    const count = Math.min(rawCount, MAX)
    report.summary.total = rawCount
    report.summary.cappedAt = MAX

    for (let i = 0; i < count; i++) {
      let r = await tryInteraction(i)
      // 过滤间歇性 500：API 失败时重试一次（仅对硬性错误 5xx/401/403 重试；400/404/422 属预期校验/缺失，不重试）
      const hard = (a) => a.status >= 500 || a.status === 401 || a.status === 403
      const apiFlaky = r.apiCalls.some(hard)
      const realCe = r.consoleErrors.some(isRealConsoleErr)
      const ceFlaky = r.consoleErrors.length > 0 && !realCe
      if (apiFlaky || (realPageErrors(r.pageErrors).length > 0)) {
        const r2 = await tryInteraction(i)
        // 仅当重试仍失败才视为真实问题
        const stillBad = r2.apiCalls.some((a) => !a.ok) || realPageErrors(r2.pageErrors).length > 0 || r2.consoleErrors.some(isRealConsoleErr)
        r.retried = true
        r.retryStillBad = stillBad
        if (!stillBad) {
          r = r2
          r.retried = true
        } else {
          r.secondApiCalls = r2.apiCalls
        }
      } else if (ceFlaky) {
        // 仅有良性 console 警告，不计入问题
        r.consoleErrors = []
      }
      const hasIssue =
        realPageErrors(r.pageErrors).length > 0 ||
        r.apiCalls.some(hard) ||
        r.consoleErrors.some(isRealConsoleErr) ||
        (r.uiError && !/不能为空|必填|required|验证|validation|导入数据不能为空|请先购买|Copy failed|请输入|please enter|is required|enter a name/i.test(r.uiError))
      if (hasIssue) {
        const reason = []
        const rpe = realPageErrors(r.pageErrors)
        if (rpe.length) reason.push('pageError:' + rpe.join(' | ').slice(0, 200))
        if (r.apiCalls.some(hard)) reason.push('apiError(' + r.apiCalls.filter(hard).map((a) => a.method + ' ' + a.url + ' ' + a.status).join(';') + ')')
        if (r.consoleErrors.some(isRealConsoleErr)) reason.push('consoleError:' + r.consoleErrors.join(' | ').slice(0, 200))
        if (r.uiError) reason.push('uiError:' + r.uiError)
        r.issue = true
        r.issueReason = reason.join(' / ')
        report.summary.issues.push({ index: i, text: r.text, reason: r.issueReason })
      }
      report.interactions.push(r)
    }

    // 仅当「初始加载那批调用」本身出错才记为页面级问题（避免把交互期调用误判为初始错误）
    const initialApiBad = apiCalls.slice(0, initialApiCount).filter((a) => a.status >= 500 || a.status === 401 || a.status === 403 || a.status === 404)
    const initialPeBad = realPageErrors(pageErrors.slice(initialPageErr))
    const initialCeBad = consoleErrors.slice(initialConsoleErr).filter(isRealConsoleErr)
    if (initialApiBad.length || initialPeBad.length || initialCeBad.length) {
      const initialIssues = []
      if (initialApiBad.length) initialIssues.push('初始API错误: ' + initialApiBad.map((a) => a.method + ' ' + a.url + ' ' + a.status).join('; '))
      if (initialPeBad.length) initialIssues.push('页面异常: ' + initialPeBad.join(' | ').slice(0, 300))
      if (initialCeBad.length) initialIssues.push('控制台错误: ' + initialCeBad.join(' | ').slice(0, 300))
      report.pageLevelIssues = initialIssues
    }
    report.loaded = true
  } catch (e) {
    report.loadError = String(e).slice(0, 500)
  } finally {
    report.finishedAt = nowIso()
    report.summary.withIssue = report.summary.issues.length + (report.pageLevelIssues ? report.pageLevelIssues.length : 0)
    report.consoleErrorsTotal = consoleErrors.length
    report.pageErrorsTotal = pageErrors.length
    report.apiCallsTotal = apiCalls.length
    writeFileSync(reportFile, JSON.stringify(report, null, 2))
    await browser.close()
  }
  if (report.loadError) process.exit(1)
  process.exit(0)
}

main().catch((e) => {
  console.error('walker 崩溃:', e)
  process.exit(3)
})
