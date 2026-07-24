import { chromium } from '@playwright/test'
const browser = await chromium.launch()
const page = await browser.newPage()
await page.goto('http://localhost:8213/#/login')
await page.waitForTimeout(2000)
await page.locator('.login-box input').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Admin@12345678')
await page.locator('.login-box button.el-button--primary').click()
await page.waitForTimeout(3000)
const data = await page.evaluate(() => {
  return {
    userInfo: localStorage.getItem('user_info'),
    pinia: window.__PINIA__ ? 'yes' : 'no',
    menuCount: document.querySelectorAll('.top-menu .el-menu-item').length
  }
})
console.log('userInfo:', data.userInfo)
console.log('menuCount:', data.menuCount)
const allMenus = await page.evaluate(() => {
  return Array.from(document.querySelectorAll('.top-menu .el-menu-item')).map(it => it.textContent.trim())
})
console.log('all menus:', allMenus)
await browser.close()
