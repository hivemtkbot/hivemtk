import { test, expect } from '@playwright/test'

const BASE = 'http://localhost:8213'

test('debug top menu', async ({ page }) => {
  test.setTimeout(30000)
  await page.addInitScript(() => {
    try { localStorage.setItem('app_locale', 'zh') } catch (e) {}
  })
  await page.goto(`${BASE}/#/login`)
  await page.waitForSelector('.login-box', { timeout: 10000 })
  await page.waitForTimeout(500)
  await page.evaluate(() => { try { localStorage.setItem('app_locale', 'zh') } catch (e) {} })
  await page.locator('.login-box input').first().fill('admin')
  await page.locator('.login-box input[type="password"]').fill('Admin@12345678')
  await page.locator('.login-box button.el-button--primary').click()
  await page.waitForTimeout(3000)

  // 调试：列出所有顶栏菜单项
  const menuItems = await page.evaluate(() => {
    // 检查所有可能的菜单项
    const allItems = document.querySelectorAll('.top-menu .el-menu-item, .el-menu--horizontal .el-menu-item')
    return Array.from(allItems).map((item, idx) => {
      const rect = item.getBoundingClientRect()
      return {
        idx,
        text: item.textContent.trim().slice(0, 50),
        x: rect.x,
        y: rect.y,
        width: rect.width,
        height: rect.height,
        visible: rect.width > 0 && rect.height > 0
      }
    })
  })
  console.log('=== Top Menu Items ===')
  console.log(JSON.stringify(menuItems, null, 2))
  console.log('Total items:', menuItems.length)

  // 列出所有 top-menu 元素
  const allTopMenus = await page.evaluate(() => {
    const tms = document.querySelectorAll('.top-menu')
    return Array.from(tms).map((tm, idx) => ({
      idx,
      tag: tm.tagName,
      innerHTML: tm.innerHTML.slice(0, 500),
      childCount: tm.children.length
    }))
  })
  console.log('=== All .top-menu elements ===')
  console.log(JSON.stringify(allTopMenus, null, 2))

  // 列出 top-menu 容器信息
  const containerInfo = await page.evaluate(() => {
    const tm = document.querySelector('.top-menu')
    if (!tm) return null
    const rect = tm.getBoundingClientRect()
    return {
      clientWidth: tm.clientWidth,
      scrollWidth: tm.scrollWidth,
      scrollLeft: tm.scrollLeft,
      rectX: rect.x,
      rectWidth: rect.width,
      innerWidth: window.innerWidth
    }
  })
  console.log('=== Top Menu Container ===')
  console.log(JSON.stringify(containerInfo, null, 2))
})
