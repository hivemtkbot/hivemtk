import { http } from '@/utils/request'

/**
 * 客户 360 拆分 API（USR-CM-05）
 * 性能优化：原 getCustomer360 单次聚合，改为并行 4 个独立 API
 * 配合 AbortController 可在切换客户时取消旧请求
 */

export const getCustomerBasic = (id, signal) =>
  http.get(`/api/customer-360/basic?user_id=${id}`, undefined, { signal })

export const getCustomerEvents = (id, params, signal) =>
  http.get(`/api/customer-360/events`, { user_id: id, ...params }, { signal })

export const getCustomerSessions = (id, params, signal) =>
  http.get(`/api/customer-360/sessions`, { user_id: id, ...params }, { signal })

export const getCustomerTags = (id, signal) =>
  http.get(`/api/customer-360/tags?user_id=${id}`, undefined, { signal })

export const getCustomerNotes = (id, signal) =>
  http.get(`/api/customer-360/notes?user_id=${id}`, undefined, { signal })

export const getCustomerOrders = (id, params, signal) =>
  http.get(`/api/customer-360/orders`, { user_id: id, ...params }, { signal })

/**
 * 加载客户 360 全量数据（并行 + 可取消）
 * @param {string} id
 * @param {object} options
 * @param {AbortSignal} options.signal - 切换客户时取消旧请求
 */
export const loadCustomer360 = async (id, { signal } = {}) => {
  const results = await Promise.allSettled([
    getCustomerBasic(id, signal),
    getCustomerEvents(id, { limit: 50 }, signal),
    getCustomerSessions(id, { limit: 20 }, signal),
    getCustomerTags(id, signal),
    getCustomerOrders(id, { limit: 20 }, signal)
  ])
  const [basic, events, sessions, tags, orders] = results
  return {
    basic: basic.status === 'fulfilled' ? basic.value : null,
    events: events.status === 'fulfilled' ? events.value : [],
    sessions: sessions.status === 'fulfilled' ? sessions.value : [],
    tags: tags.status === 'fulfilled' ? tags.value : [],
    orders: orders.status === 'fulfilled' ? orders.value : [],
    errors: results.filter((r) => r.status === 'rejected').map((r) => r.reason?.message)
  }
}

// 客户 360 Pinia store 缓存 key 前缀
export const CACHE_KEY_PREFIX = 'c360:'

export const buildCacheKey = (id) => `${CACHE_KEY_PREFIX}${id}`

export const getCustomerList = (params) =>
  http.get('/api/customers', params)

export const getCustomerDetail = (id) =>
  http.get(`/api/customers/${id}`)

export const addCustomerTag = (id, tag) =>
  http.post(`/api/customers/${id}/tags`, { tag })

export const removeCustomerTag = (id, tag) =>
  http.delete(`/api/customers/${id}/tags/${encodeURIComponent(tag)}`)
