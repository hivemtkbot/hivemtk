/**
 * P2-5 订单管理 E2E 测试
 */
import { test, expect, request } from '@playwright/test'

const API_BASE = 'http://localhost:8204'

async function getAdminToken() {
  const ctx = await request.newContext({ baseURL: API_BASE })
  const resp = await ctx.post('/api/auth/login', {
    data: { username: 'admin', password: 'Admin@12345678' }
  })
  const body = await resp.json()
  return body.data?.token
}

test.describe('P2-5 订单管理 E2E', () => {
  test('API: 订单列表分页', async () => {
    const token = await getAdminToken()
    const ctx = await request.newContext({
      baseURL: API_BASE,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    })
    const resp = await ctx.get('/api/orders/list', { params: { page: 1, page_size: 10 } })
    expect(resp.ok()).toBeTruthy()
    const body = await resp.json()
    expect(body.code).toBe('SUCCESS')
    expect(Array.isArray(body.data.list)).toBe(true)
  })

  test('API: 创建订单 → 详情 → 取消 全流程', async () => {
    const token = await getAdminToken()
    const ctx = await request.newContext({
      baseURL: API_BASE,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    })
    // 1) 创建订单
    const createResp = await ctx.post('/api/orders', {
      data: { account_id: 'e2e_acc', tg_id: 99999, price: '199.00' }
    })
    expect(createResp.ok()).toBeTruthy()
    const createBody = await createResp.json()
    expect(createBody.code).toBe('SUCCESS')
    const orderId = createBody.data.id
    expect(orderId).toBeTruthy()

    // 2) 查详情
    const detailResp = await ctx.get(`/api/orders/${orderId}`)
    expect(detailResp.ok()).toBeTruthy()
    const detailBody = await detailResp.json()
    expect(detailBody.data.id).toBe(orderId)

    // 3) 取消订单
    const cancelResp = await ctx.post(`/api/orders/${orderId}/cancel`, {
      data: { reason: 'e2e 测试取消' }
    })
    expect(cancelResp.ok()).toBeTruthy()
    const cancelBody = await cancelResp.json()
    expect(cancelBody.code).toBe('SUCCESS')
  })

  test('API: 已支付订单才能退款', async () => {
    const token = await getAdminToken()
    const ctx = await request.newContext({
      baseURL: API_BASE,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    })
    // 创建新订单 (status=0 待支付)
    const createResp = await ctx.post('/api/orders', {
      data: { account_id: 'e2e_acc2', tg_id: 88888, price: '50.00' }
    })
    const orderId = (await createResp.json()).data.id
    // 待支付订单退款应失败
    const refundResp = await ctx.post(`/api/orders/${orderId}/refund`, {
      data: { amount: '50', reason: '测试' }
    })
    // 业务层应返回非 200 或 4xx（视具体实现）
    expect(refundResp.status()).toBeGreaterThanOrEqual(400)
  })

  test('API: 校验创建订单必填字段', async () => {
    const token = await getAdminToken()
    const ctx = await request.newContext({
      baseURL: API_BASE,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    })
    // 缺 tg_id
    const resp = await ctx.post('/api/orders', {
      data: { account_id: 'a', price: '10' }
    })
    expect(resp.status()).toBeGreaterThanOrEqual(400)
  })
})
