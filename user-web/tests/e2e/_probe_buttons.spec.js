// 快速探测 - 看哪些按钮能正常看到
import { test, expect } from '@playwright/test'

const BASE = process.env.E2E_BASE_URL || 'http://localhost:8211'
const API = process.env.E2E_API_URL || 'http://localhost:8204'
const ADMIN = 'admin'
const ADMIN_PW = 'Admin@12345678'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

let TOKEN = ''
async function refreshToken() {
  for (let i = 0; i < 4; i++) {
    try {
      const resp = await fetch(API + '/api/auth/login', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: ADMIN, password: ADMIN_PW })
      })
      const b = await resp.json().catch(() => ({}))
      const t = b?.data?.token || b?.token || ''
      if (t) { TOKEN = t; return t }
    } catch (e) { /* retry */ }
    await sleep(1000)
  }
  return ''
}

test('probe buttons', async ({ page }) => {
  await refreshToken()
  // 跟主测试一样：先到根，强制 locale=zh-cn，再去登录
  await page.goto(BASE, { waitUntil: 'domcontentloaded' })
  await page.evaluate(() => localStorage.setItem('app_locale', 'zh'))
  await page.goto(BASE + '/#/login', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(800)
  // 探测中英文情况
  const h2 = await page.locator('.login-box h2').first().innerText().catch(() => 'NO_H2')
  const btn = await page.locator('.login-box .el-button--primary').first().innerText().catch(() => 'NO_BTN')
  console.log(`[login] h2="${h2}" btn="${btn}"`)
  await page.waitForSelector('.login-box input', { timeout: 20000 })
  await page.locator('.login-box input').nth(0).fill(ADMIN)
  await page.locator('.login-box input[type="password"]').fill(ADMIN_PW)
  await page.locator('.login-box .el-button--primary').click()
  await page.waitForSelector('.el-menu', { timeout: 25000 })

  for (const [route, sel] of [
    ['clue/list', '.el-main button:has-text("导入线索")'],
    ['clue/list', '.clue-list-page .el-button--primary'],
    ['tagSegmentation/list', '.el-main button:has-text("新增标签")'],
    ['tagSegmentation/list', '.tag-segmentation-page .el-button--primary'],
    ['userSegment/list', '.el-main button:has-text("创建分群")'],
    ['userSegment/list', '.user-segment-page .el-button--primary'],
    ['oneid/list', '.oneid-list-container button:has-text("解析/创建 OneID")'],
    ['customer360/list', '.customer-360-page .el-button--primary']
  ]) {
    await page.evaluate((h) => { window.location.hash = '#/' + h }, route)
    await page.waitForTimeout(2500)
    const c = await page.locator(sel).count()
    const vis = await page.locator(sel).first().isVisible().catch(() => false)
    console.log(`[${route}] sel="${sel}" count=${c} visible=${vis}`)
  }
})
