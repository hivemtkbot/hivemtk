/**
 * 订单 API 单元测试
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/utils/request', () => ({ default: request }))

import {
  getOrderList,
  getOrderByID,
  createOrder,
  cancelOrder,
  refundOrder,
  payOrder,
  checkPayStatus,
  getRecentOrderList,
  updateOrder,
  deleteOrder
} from '@/api/order.js'

describe('订单 API', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ code: 'SUCCESS', data: {} })
  })

  it('getOrderList 调用 /api/orders/list', async () => {
    await getOrderList({ page: 1, page_size: 20 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/orders/list', method: 'get' })
    )
  })

  it('getOrderByID 拼接 ID', async () => {
    await getOrderByID('order_abc')
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/orders/order_abc', method: 'get' })
    )
  })

  it('createOrder 提交 body', async () => {
    await createOrder({ account_id: 'a1', tg_id: 123, price: '99.00' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/orders',
        method: 'post',
        data: { account_id: 'a1', tg_id: 123, price: '99.00' }
      })
    )
  })

  it('cancelOrder 传 reason', async () => {
    await cancelOrder('o1', '客户取消')
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/orders/o1/cancel',
        method: 'post',
        data: { reason: '客户取消' }
      })
    )
  })

  it('refundOrder 传金额和原因', async () => {
    await refundOrder('o1', { amount: '99', reason: '重复下单' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/orders/o1/refund',
        method: 'post',
        data: { amount: '99', reason: '重复下单' }
      })
    )
  })

  it('payOrder 路径与请求体正确', async () => {
    await payOrder({ id: 'o1', account_id: 'a', tg_id: 1, price: '1' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/order/o1/pay',
        method: 'post'
      })
    )
  })

  it('checkPayStatus 调用 GET', async () => {
    await checkPayStatus('o1')
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/order/o1/check-pay', method: 'get' })
    )
  })

  it('getRecentOrderList 调用 /api/orders/recent', async () => {
    await getRecentOrderList({ limit: 10 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/orders/recent', method: 'get' })
    )
  })

  it('updateOrder 调用 PUT', async () => {
    await updateOrder('o1', { price: '100' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/order/o1', method: 'put' })
    )
  })

  it('deleteOrder 调用 DELETE', async () => {
    await deleteOrder('o1')
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/orders/o1', method: 'delete' })
    )
  })
})
