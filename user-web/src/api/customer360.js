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

// v3 审计 P1 修复：原 /api/customers* 四个端点后端不存在（404）。
// 统一改接已注册的 /api/customer-360/* 路由；标签增删用「读-合并-全量写」
// 适配后端的 PUT 全量替换契约。

export const getCustomerList = (params) =>
  http.get('/api/customer-360/list', params)

export const getCustomerBasicById = (id) =>
  http.get(`/api/customer-360/basic?user_id=${encodeURIComponent(id)}`)

export const getCustomerDetail = async (id) => {
  // 聚合出旧版详情结构（basic_info/tags 等）；消息时间线由会话接口另行加载，
  // 此处返回空数组占位以保持 List.vue 的字段映射兼容
  const settled = await Promise.allSettled([
    getCustomerBasicById(id),
    getCustomerTags(id)
  ])
  const basic = settled[0].status === 'fulfilled' ? settled[0].value : null
  const tags = settled[1].status === 'fulfilled' ? settled[1].value : []
  return {
    ...(basic && typeof basic === 'object' ? basic : {}),
    basic_info: basic?.basic_info || basic?.basic_info === null ? (basic?.basic_info ?? basic) : basic,
    tags: Array.isArray(tags) ? tags : tags?.tags || [],
    message_history: []
  }
}

const fetchTagsRaw = async (id) => {
  const r = await http.get(`/api/customer-360/tags?user_id=${encodeURIComponent(id)}`)
  if (Array.isArray(r)) return r
  if (Array.isArray(r?.tags)) return r.tags
  return []
}

export const addCustomerTag = async (id, tag) => {
  const tags = await fetchTagsRaw(id)
  if (!tags.includes(tag)) tags.push(tag)
  return http.put(`/api/customer-360/tags?user_id=${encodeURIComponent(id)}`, { tags })
}

export const removeCustomerTag = async (id, tag) => {
  const tags = (await fetchTagsRaw(id)).filter((t) => t !== tag)
  return http.put(`/api/customer-360/tags?user_id=${encodeURIComponent(id)}`, { tags })
}
