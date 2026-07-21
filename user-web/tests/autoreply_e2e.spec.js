import { test, expect, request } from '@playwright/test'

const API_BASE = 'http://localhost:8204'

const ROLES = [
  { role: 'admin', username: 'admin', password: 'Admin@123456', canAccess: true },
  { role: 'manager', username: 'test_manager', password: 'manager123', canAccess: true },
  { role: 'viewer', username: 'test_viewer', password: 'viewer123', canAccess: true }
]

async function getToken(username, password) {
  const context = await request.newContext({ baseURL: API_BASE })
  const resp = await context.post('/api/auth/login', {
    data: { username, password }
  })
  const body = await resp.json()
  return body.data?.token
}

async function apiContext(token) {
  return request.newContext({
    baseURL: API_BASE,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` }
  })
}

async function prepareRagProduct(ctx) {
  const resp = await ctx.post('/api/rag-config/products', {
    data: {
      name: 'E2E测试RAG产品',
      description: '用于UI自动化测试',
      config: {}
    }
  })
  if (!resp.ok()) return null
  const body = await resp.json()
  return body.data?.id || null
}

async function cleanupRagProduct(ctx, id) {
  if (!id) return
  await ctx.delete(`/api/rag-config/products/${id}`).catch(() => {})
}

async function ensureTestUser(ctx, username, password, role) {
  // 先尝试登录，若成功则无需创建
  const probe = await request.newContext({ baseURL: API_BASE })
  const loginResp = await probe.post('/api/auth/login', {
    data: { username, password }
  })
  if (loginResp.ok()) {
    const body = await loginResp.json()
    return body.data?.user?.id || null
  }

  // 创建用户
  const createResp = await ctx.post('/api/team/users', {
    data: { username, password, name: username, role }
  })
  if (!createResp.ok()) {
    console.warn(`创建测试用户 ${username} 失败`, await createResp.text())
    return null
  }
  const body = await createResp.json()
  return body.data?.id || null
}

test.describe('AutoReply 抖音自动回复页面', () => {
  let adminToken
  let ctx
  let testProductId
  const testUserIds = {}

  test.beforeAll(async () => {
    adminToken = await getToken('admin', 'Admin@123456')
    ctx = await apiContext(adminToken)
    testProductId = await prepareRagProduct(ctx)

    for (const { role, username, password } of ROLES) {
      if (username === 'admin') continue
      testUserIds[username] = await ensureTestUser(ctx, username, password, role)
    }
  })

  test.afterAll(async () => {
    await cleanupRagProduct(ctx, testProductId)
    for (const id of Object.values(testUserIds)) {
      if (id) await ctx.delete(`/api/team/users/${id}`).catch(() => {})
    }
    await ctx?.dispose()
  })

  test('管理员登录并通过菜单进入抖音自动回复页面，可保存规则与启停', async ({ page }) => {
    // 1. 登录
    await page.goto('/#/login')
    await expect(page.locator('input[placeholder="用户名"]')).toBeVisible()
    await page.fill('input[placeholder="用户名"]', 'admin')
    await page.fill('input[placeholder="密码"]', 'Admin@123456')
    await page.click('button:has-text("登录")')
    await expect(page).not.toHaveURL(/\/login/)

    // 2. 通过顶部菜单 + 左侧子菜单导航到抖音自动回复
    await page.click('.top-menu .el-menu-item:has-text("触达运营")')
    await page.click('.aside-content .el-sub-menu:has-text("抖音")')
    await page.click('.aside-content .el-menu-item:has-text("抖音自动回复")')
    await expect(page).toHaveURL(/\/douyin\/auto-reply/)

    // 3. 验证页面关键元素存在
    await expect(page.locator('text=自动回复账号')).toBeVisible()
    await expect(page.locator('text=回复规则')).toBeVisible()
    await expect(page.locator('text=最近回复日志')).toBeVisible()

    // 4. 保存规则（关键词+话术）
    await page.fill('.auto-reply textarea >> nth=0', 'e2e关键词')
    await page.fill('.auto-reply textarea >> nth=1', 'e2e回复话术')
    await page.fill('input[type="number"] >> nth=0', '45')
    await page.fill('input[type="number"] >> nth=1', '80')

    // 启用 RAG 并选择产品（RAG 产品 ID 与规则字段类型一致时才可选）
    if (testProductId) {
      const ragSwitch = page.locator('.auto-reply .el-switch').nth(1)
      if (await ragSwitch.isVisible().catch(() => false)) {
        await ragSwitch.click()
        await page.click('.auto-reply .el-select')
        await page.click(`.el-select-dropdown__item:has-text("E2E测试RAG产品")`)
      }
    }

    await page.click('button:has-text("保存规则")')
    await page.waitForTimeout(3000)
    await expect(page.locator('.el-message--success').filter({ hasText: '保存' })).toBeVisible()

    // 5. 启动/停止自动回复
    await page.click('button:has-text("启动自动回复")')
    await page.waitForTimeout(500)
    await page.click('button:has-text("停止")')
    await page.waitForTimeout(500)

    // 6. 验证日志表格已加载（列头存在即可）
    await expect(page.locator('th:has-text("时间")').first()).toBeVisible()
    await expect(page.locator('th:has-text("目标")').first()).toBeVisible()
    await expect(page.locator('th:has-text("回复")').first()).toBeVisible()
    await expect(page.locator('th:has-text("状态")').first()).toBeVisible()
  })

  for (const { role, username, password, canAccess } of ROLES) {
    test(`角色权限检查：${role} ${canAccess ? '可以' : '不可以'}访问抖音自动回复`, async ({ page }) => {
      await page.goto('/#/login')
      await page.fill('input[placeholder="用户名"]', username)
      await page.fill('input[placeholder="密码"]', password)
      await page.click('button:has-text("登录")')
      await expect(page).not.toHaveURL(/\/login/)

      await page.click('.top-menu .el-menu-item:has-text("触达运营")')
      const douyinMenu = page.locator('.aside-content .el-sub-menu:has-text("抖音")')

      if (canAccess) {
        await expect(douyinMenu).toBeVisible()
        await douyinMenu.click()
        await page.click('.aside-content .el-menu-item:has-text("抖音自动回复")')
        await expect(page).toHaveURL(/\/douyin\/auto-reply/)
        await expect(page.locator('text=回复规则')).toBeVisible()
      } else {
        await expect(douyinMenu).toHaveCount(0)
      }
    })
  }
})
