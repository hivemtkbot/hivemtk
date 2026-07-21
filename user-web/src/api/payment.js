import { http } from '@/utils/request'

/**
 * 支付配置 API
 * 真实调用后端 /api/order/config 端点
 */
export const PaymentConfigApi = {
  // 获取支付配置
  getConfig: () => {
    return http.get('/api/order/config')
  },

  // 保存支付配置
  saveConfig: (data) => {
    return http.post('/api/order/config', data)
  }
}

/**
 * 订单管理 API
 * 真实调用后端 /api/order/* 端点
 */
export const OrderApi = {
  // 获取订单列表
  getList: (params) => {
    return http.get('/api/order/list', params)
  },

  // 获取订单详情
  getById: (id) => {
    return http.get(`/api/order/${id}`)
  },

  // 创建订单
  create: (data) => {
    return http.post('/api/order', data)
  },

  // 更新订单
  update: (id, data) => {
    return http.put(`/api/order/${id}`, data)
  },

  // 删除订单
  delete: (id) => {
    return http.delete(`/api/order/${id}`)
  },

  // 取消订单
  cancel: (id, reason) => {
    return http.post(`/api/order/${id}/cancel`, { reason })
  },

  // 退款
  refund: (id, data) => {
    return http.post(`/api/order/${id}/refund`, data)
  }
}

export default {
  PaymentConfigApi,
  OrderApi
}
