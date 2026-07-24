import { chromium } from '@playwright/test'
const browser = await chromium.launch()
const page = await browser.newPage()

await page.addInitScript(() => {
  try { localStorage.setItem('app_locale', 'zh') } catch (e) {}
})

await page.goto('http://localhost:8213/#/login')
await page.waitForSelector('.login-box', { timeout: 8000 })
await page.locator('.login-box input').first().fill('admin')
await page.locator('.login-box input[type="password"]').fill('Admin@12345678')
await page.locator('.login-box button.el-button--primary').click()
await page.waitForTimeout(3000)

const result = await page.evaluate(() => {
  // 1. 查所有顶栏菜单（包括隐藏的）
  const topMenuEl = document.querySelector('.top-menu')
  const allItems = topMenuEl ? Array.from(topMenuEl.querySelectorAll('.el-menu-item, .el-sub-menu')) : []
  const items = allItems.map(it => {
    const rect = it.getBoundingClientRect()
    const cs = getComputedStyle(it)
    return {
      tag: it.tagName,
      cls: it.className,
      text: it.textContent.trim().slice(0, 30),
      offsetWidth: it.offsetWidth,
      offsetHeight: it.offsetHeight,
      display: cs.display,
      visibility: cs.visibility,
      opacity: cs.opacity,
      position: cs.position,
      rectLeft: rect.left,
      rectRight: rect.right,
      popperVisible: it.getAttribute('aria-haspopup'),
      ariaHidden: it.getAttribute('aria-hidden')
    }
  })

  // 2. 容器尺寸
  const topMenuRect = topMenuEl ? topMenuEl.getBoundingClientRect() : null
  const innerUl = topMenuEl ? topMenuEl.querySelector('ul') : null
  const innerUlRect = innerUl ? innerUl.getBoundingClientRect() : null
  const innerUlCS = innerUl ? getComputedStyle(innerUl) : null

  return {
    itemsCount: items.length,
    items,
    topMenu: topMenuRect ? { left: topMenuRect.left, right: topMenuRect.right, width: topMenuRect.width, scrollWidth: topMenuEl.scrollWidth, scrollLeft: topMenuEl.scrollLeft } : null,
    innerUl: innerUlRect ? { left: innerUlRect.left, right: innerUlRect.right, width: innerUlRect.width, scrollWidth: innerUl.scrollWidth, scrollLeft: innerUl.scrollLeft, overflow: innerUlCS.overflow, display: innerUlCS.display } : null
  }
})

console.log('items count:', result.itemsCount)
console.log('topMenu container:', result.topMenu)
console.log('inner ul:', result.innerUl)
console.log('all items:')
result.items.forEach((m, i) => console.log(`  [${i}] ${JSON.stringify(m)}`))

await browser.close()
