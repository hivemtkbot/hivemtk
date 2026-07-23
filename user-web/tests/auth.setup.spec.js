import { test, expect } from '@playwright/test'

const BASE = process.env.E2E_BASE_URL || 'http://localhost:8213'
const STATE = 'tests/.auth/user.json'

test.use({ baseURL: BASE })

test('authenticate admin and persist storage state', async ({ page }) => {
  await page.goto('/#/login')
  await page.waitForSelector('.login-box', { timeout: 15000 })
  await page.locator('.login-box input[type="text"]').first().fill('admin')
  await page.locator('.login-box input[type="password"]').fill('Admin@123456')
  await page.locator('.login-box button.el-button--primary').click()
  await expect(page).not.toHaveURL(/\/login/, { timeout: 10000 })
  await page.context().storageState({ path: STATE })
})
