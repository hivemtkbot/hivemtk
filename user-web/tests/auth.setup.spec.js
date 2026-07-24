import { test, expect } from '@playwright/test'
import path from 'path'

const BASE = process.env.E2E_BASE_URL || 'http://localhost:8213'
const STATE = path.resolve(process.cwd(), 'tests/.auth/user.json')

// 候选密码：覆盖历史重置值(Admin@123456)与平台代理凭证(PLATFORM_ADMIN_PASSWORD 环境变量)，
// 逐一尝试直到登录成功，避免容器重建后凭据失效导致整套测试无法运行。
const CANDIDATES = [
  'Admin@12345678',
  'Admin@123456',
  '62cfdc6bf1b075830734cc6f9a63501b'
]

test.use({ baseURL: BASE })

test('authenticate admin and persist storage state', async ({ page }) => {
  await page.goto('/#/login')
  await page.waitForSelector('.login-box', { timeout: 15000 })

  let ok = false
  for (const pw of CANDIDATES) {
    await page.locator('.login-box input[type="text"]').first().fill('admin')
    await page.locator('.login-box input[type="password"]').fill(pw)
    await page.locator('.login-box button.el-button--primary').click()
    try {
      // 登录成功后 SPA 会离开 /login（hash 不再含 /login）
      await page.waitForURL((url) => !url.hash.includes('/login'), { timeout: 6000 })
      ok = true
      break
    } catch (e) {
      await page.waitForTimeout(400)
    }
  }
  expect(ok, '所有候选密码均无法登录，请检查管理员凭据').toBeTruthy()
  await page.context().storageState({ path: STATE })
})
