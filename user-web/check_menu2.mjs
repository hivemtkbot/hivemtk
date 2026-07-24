import { chromium } from '@playwright/test'
const browser = await chromium.launch()
const page = await browser.newPage()

// 强制 zh locale
await page.addInitScript(() => {
  try { localStorage.setItem('app_locale', 'zh') } catch (e) {}
})

await page.goto('http://localhost:8213/#/login')
await page.waitForTimeout(2000)
console.log('currentURL:', page.url())

const userInfo1 = await page.evaluate(() => localStorage.getItem('user_info'))
console.log('userInfo pre-login:', userInfo1)

// 等待登录表单
await page.waitForSelector('.login-box', { timeout: 8000 })
await page.locator('.login-box input').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Admin@12345678')
await page.locator('.login-box button.el-button--primary').click()
await page.waitForTimeout(3000)
console.log('currentURL after login:', page.url())

const userInfo2 = await page.evaluate(() => localStorage.getItem('user_info'))
console.log('userInfo post-login:', userInfo2)

await page.waitForTimeout(2000)

const data = await page.evaluate(() => {
  const items = document.querySelectorAll('.top-menu .el-menu-item')
  return {
    menuCount: items.length,
    menus: Array.from(items).map(it => ({
      text: it.textContent.trim(),
      display: getComputedStyle(it).display,
      visibility: getComputedStyle(it).visibility,
      width: it.offsetWidth,
      height: it.offsetHeight,
      opacity: getComputedStyle(it).opacity
    }))
  }
})
console.log('menuCount:', data.menuCount)
console.log('all menus:')
data.menus.forEach((m, i) => console.log(`  [${i}] text=${JSON.stringify(m.text)} display=${m.display} visibility=${m.visibility} w=${m.width} h=${m.height} opacity=${m.opacity}`))

// 滚动到最右
await page.evaluate(() => {
  const tm = document.querySelector('.top-menu .el-menu, .top-menu')
  if (tm) tm.scrollLeft = tm.scrollWidth
})
await page.waitForTimeout(500)
const data2 = await page.evaluate(() => {
  const items = document.querySelectorAll('.top-menu .el-menu-item')
  return Array.from(items).map(it => it.textContent.trim())
})
console.log('menus after scrollRight:')
data2.forEach((m, i) => console.log(`  [${i}] ${JSON.stringify(m)}`))

await browser.close()
