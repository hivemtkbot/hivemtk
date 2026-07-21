import { test, expect } from '@playwright/test'

/**
 * 冒烟测试 - 验证 user-web 基础可用性
 * 不使用 mock 数据，直接访问真实前端 + 后端
 */

test.describe('user-web 冒烟测试', () => {
  test('首页可加载', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')
    const body = page.locator('body')
    await expect(body).toBeVisible()
  })

  test('登录页可访问', async ({ page }) => {
    await page.goto('/#/login')
    await page.waitForLoadState('networkidle')
    const pageContent = await page.locator('body').innerText()
    expect(pageContent.length).toBeGreaterThan(0)
  })

  test('初始化页可访问', async ({ page }) => {
    await page.goto('/#/setup')
    await page.waitForLoadState('networkidle')
    const pageContent = await page.locator('body').innerText()
    expect(pageContent.length).toBeGreaterThan(0)
  })

  test('OneID 路由模块已注册 (T5 P-01 验证)', async ({ page }) => {
    // 验证 oneid 路由模块已注册 (P-01 修复验证)
    // 未登录时应跳转到 /login 或 /setup，但不应出现 404
    await page.goto('/#/oneid/list')
    await page.waitForLoadState('networkidle')
    // 不应出现 "页面不存在" 的 NotFound 页面
    const bodyText = await page.locator('body').innerText()
    expect(bodyText).not.toContain('页面不存在')
  })
})
