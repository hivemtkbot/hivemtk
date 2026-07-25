// 诊断：登录后进入指定路由，抓取 console/pageerror 与关键 DOM
import { chromium } from 'playwright'
const route = process.argv[2] || '/notifications'
const base = process.argv[3] || 'http://127.0.0.1:8211'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
const page = await browser.newContext({ viewport: { width: 1440, height: 900 } }).then((c) => c.newPage())
const ces = [], pes = [], apis = []
page.on('console', (m) => { if (m.type() === 'error') ces.push(m.text().slice(0, 400)) })
page.on('pageerror', (e) => pes.push((e.stack || String(e)).slice(0, 600)))
page.on('response', (r) => { const u = r.url(); if (u.includes('/api')) apis.push(r.request().method() + ' ' + u.replace(base, '') + ' -> ' + r.status()) })
await page.goto(base + '/#/login', { waitUntil: 'load', timeout: 30000 })
await page.waitForSelector('.el-input input', { timeout: 10000 })
const ins = await page.$$('.el-input input')
await ins[0].fill('admin'); await ins[1].fill('Seed@123456')
await page.click('button.el-button--primary', { timeout: 10000 })
await page.waitForFunction(() => !location.hash.includes('/login'), { timeout: 15000 }).catch(() => {})
await page.evaluate((p) => { location.hash = '#' + p }, route)
await sleep(5000)
const diag = await page.evaluate(() => ({
  hash: location.hash,
  title: document.title,
  hasAppMain: !!document.querySelector('.app-main'),
  appMainHTML: (document.querySelector('.app-main')?.innerHTML || '').slice(0, 400),
  bodyText: document.body.innerText.slice(0, 300),
}))
console.log('hash:', diag.hash)
console.log('title:', diag.title)
console.log('hasAppMain:', diag.hasAppMain)
console.log('appMainHTML:', diag.appMainHTML)
console.log('bodyText:', diag.bodyText)
console.log('API calls during load:', JSON.stringify(apis, null, 1))
console.log('consoleErrors:', JSON.stringify(ces, null, 1))
console.log('pageErrors:', JSON.stringify(pes, null, 1))
await browser.close()
