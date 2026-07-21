import request from '@/utils/request'

// 订单列表
export function getOrderList(params) {
  return request({ url: '/api/orders/list', method: 'get', params })
}

// 订单详情
export function getOrderByID(id) {
  return request({ url: `/api/orders/${id}`, method: 'get' })
}

// 创建订单
export function createOrder(data) {
  return request({ url: '/api/orders', method: 'post', data })
}

// 取消订单
export function cancelOrder(id, reason) {
  return request({ url: `/api/orders/${id}/cancel`, method: 'post', data: { reason } })
}

// 订单退款
export function refundOrder(id, data) {
  return request({ url: `/api/orders/${id}/refund`, method: 'post', data })
}

// 支付订单
export function payOrder(data) {
  return request({ url: `/api/order/${data.id}/pay`, method: 'post', data })
}

// 查询支付状态
export function checkPayStatus(id) {
  return request({ url: `/api/order/${id}/check-pay`, method: 'get' })
}

// 最近订单
export function getRecentOrderList(params) {
  return request({ url: '/api/orders/recent', method: 'get', params })
}

// 更新订单
export function updateOrder(id, data) {
  return request({ url: `/api/order/${id}`, method: 'put', data })
}

// 删除订单
export function deleteOrder(id) {
  return request({ url: `/api/orders/${id}`, method: 'delete' })
}
