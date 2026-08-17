/**
 * 统一 HTTP 请求工具
 *
 * 导出两种风格，**新代码必须使用命名导入 `http`**：
 *   ✅ import { http } from '@/utils/request'
 *      http.get(url, params) / http.post(url, data) / http.put / http.delete / http.upload
 *   ⚠️  import request from '@/utils/request'   // default 导出，仅为兼容旧代码保留
 *      request.get(url, { params }) / request.post(url, data, config)
 *
 * 选择 `http` 的原因：
 *   1. `http.get(url, params)` 直接传扁平参数，无需嵌套 `{ params }`，避免误写成
 *      `http.get(url, { params })` 导致参数二次包裹（见 http.get 兜底注释）。
 *   2. `http` 是稳定的语义 API（get/post/put/delete/upload），未来若替换底层
 *      axios 实例（如改用 fetch/ofetch），调用方零改动；而 `request` 直接暴露
 *      axios 实例，与 axios API 强耦合。
 * 3. 架构评审 约定：`default` 导出仅作向后兼容，新增文件应使用 `{ http }`。
 *      ESLint `no-restricted-imports` 规则可约束新增文件不引入 default 导出。
 *
 * 历史背景：早期 api 文件混用两种风格（43 个文件用 default，33 个用 { http }，另 3 个未导入），
 * 全量回归改造风险高、收益低；故仅以注释 + lint 规则约束新增，存量保持现状。
 */
import axios from 'axios'
import { ElMessage } from 'element-plus'
import { getApiConfig } from './configManager'
import i18n from '@/i18n'
import { getStoredLocale } from '@/i18n/locale'

const t = (key) => i18n.global.t(key)

// 创建axios实例（不使用 X-API-KEY）
const createRequestInstance = () => {
  // 优先使用环境变量（Vite 注入），然后是本地存储，最后是默认值
  let apiBaseUrl = import.meta.env?.VITE_API_BASE_URL || ''
  try {
    const configStr = localStorage.getItem('apiConfig')
    if (configStr) {
      const cfg = JSON.parse(configStr)
      apiBaseUrl = cfg.baseUrl || apiBaseUrl
    }
  } catch (e) {}
  return axios.create({
    baseURL: apiBaseUrl,
    headers: {
      'Content-Type': 'application/json'
    }
  })
}

// ============================================================
// 统一报错处理工具
// ============================================================

// 特殊业务码：后端以 2xx 返回，需要前端跳转（见 InitGuard 中间件）
// 开源版：已移除 INIT_PASSWORD_REQUIRED / LICENSE_*（授权与强制改密流程已取消）
const INIT_REDIRECT_MAP = {
  INIT_REQUIRED: '/setup'
}

// 简单的 toast 去重，避免并发请求同时弹出多条相同提示
let lastToastMsg = ''
let lastToastTs = 0
function showToast(message, type = 'error') {
  const msg = message || t('http.requestFailed')
  const now = Date.now()
  if (msg === lastToastMsg && now - lastToastTs < 2500) return
  lastToastMsg = msg
  lastToastTs = now
  if (type === 'warning') ElMessage.warning(msg)
  else if (type === 'info') ElMessage.info(msg)
  else ElMessage.error(msg)
}

// 判断响应是否为 JSON（反向代理把 404 兜底成前端 HTML 时不是 JSON）
function isJsonResponse(resp) {
  const ct =
    (resp && resp.headers && (resp.headers['content-type'] || resp.headers['Content-Type'])) || ''
  return String(ct).toLowerCase().includes('application/json')
}

// 从响应体提取后端业务消息（优先 message，其次 msg）
function extractServerMessage(data, fallback) {
  if (data && typeof data === 'object') {
    return data.message || data.msg || fallback
  }
  return fallback
}

// 构造统一错误对象，保留 response 与业务码，便于调用方分支处理
function buildRequestError(message, response, bizCode) {
  const err = new Error(message || t('http.requestFailed'))
  err.response = response || null
  if (bizCode !== undefined) err.bizCode = bizCode
  return err
}

// 统一跳转：优先 vue-router（避免整页刷新），失败回退 hash 跳转
function redirectTo(path) {
  if (!path) return
  const target = path.startsWith('/') ? path : '/' + path
  const current = (window.location.hash || '').replace(/^#/, '') || '/'
  if (current === target) return
  import('@/router')
    .then((m) => {
      const r = m.default
      if (r && typeof r.push === 'function') r.push(target)
      else window.location.href = '#' + target
    })
    .catch(() => {
      window.location.href = '#' + target
    })
}

// 清除登录态并跳登录页
function clearAuthAndGoLogin() {
  localStorage.removeItem('token')
  localStorage.removeItem('refreshToken')
  redirectTo('/login')
}

// 添加拦截器
const addInterceptors = () => {
  // 请求拦截器
  request.interceptors.request.use(
    (config) => {
      // 添加token到请求头
      const token = localStorage.getItem('token')
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      // 透传当前语言，供后端返回本地化业务提示
      config.headers['Accept-Language'] = getStoredLocale()
      return config
    },
    (error) => {
      console.error('请求错误:', error)
      return Promise.reject(error)
    }
  )

  // 响应拦截器
  request.interceptors.response.use(
    (response) => {
      const config = response.config
      let data = response.data
      const silent = !!(config && config._silent)

      // 0) Blob 下载响应（导出文件等）：responseType 为 blob 时 axios 已返回二进制，
      // 不做业务码/JSON 解析，直接透传 Blob 供调用方下载（否则导出会报“非 JSON 响应”）。
      if (config && config.responseType === 'blob') {
        return response.data
      }

      // 1) 非 JSON 响应（如反向代理把 404 兜底成前端 HTML 页面）
      //    容错：部分后端接口 Content-Type 非 application/json（如 text/plain），
      //    但响应体本身是合法 JSON，应正常解析，避免误判为"非 JSON 响应"。
      let body = data
      if (!isJsonResponse(response) && typeof data === 'string') {
        try {
          const parsed = JSON.parse(data)
          if (parsed && typeof parsed === 'object') {
            body = parsed
          }
        } catch {
          // 解析失败说明确为非 JSON（如 HTML 兜底页），保持原样交给下方判断
        }
      }
      if (!isJsonResponse(response) && typeof body !== 'object') {
        if (import.meta.env?.DEV) {
          console.error('[request] 收到非 JSON 响应，疑似网关/Nginx 兜底到前端页面：', data)
        }
        const msg = t('http.unexpectedResponse')
        if (!silent) showToast(msg)
        return Promise.reject(buildRequestError(msg, response))
      }
      data = body

      // 2) 响应体为空
      if (data == null) {
        const msg = t('http.emptyResponse')
        if (!silent) showToast(msg)
        return Promise.reject(buildRequestError(msg, response))
      }

      const code = data.code

      // 3) 成功（兼容 user-server 的 "SUCCESS" 与 platform-server 的 200 / 0）
      if (code === 'SUCCESS' || code === 200 || code === 0) {
        return data.data
      }

      // 4) 需要跳转的特殊业务码（后端以 2xx 返回）
      if (INIT_REDIRECT_MAP[code]) {
        const target = data.redirect || INIT_REDIRECT_MAP[code]
        redirectTo(target)
        const msg = data.message || t('http.initRequired')
        if (!silent) showToast(msg, 'warning')
        return Promise.reject(buildRequestError(msg, response, code))
      }

      // 5) 其它业务错误：弹统一提示
      const msg = extractServerMessage(data, t('http.requestFailed'))
      if (!silent) showToast(msg)
      return Promise.reject(buildRequestError(msg, response, code))
    },
    (error) => {
      if (error.response) {
        const { status, data, config } = error.response
        const silent = !!(config && config._silent)

        // 非 JSON 兜底响应（如 init-license 路由缺失时 Nginx 返回 404 HTML）
        if (!isJsonResponse(error.response) || typeof data !== 'object') {
          if (import.meta.env?.DEV) {
            console.error('[request] 网关返回非 JSON 响应：', data)
          }
          const msg = t('http.unexpectedResponse')
          if (!silent) showToast(msg)
          return Promise.reject(buildRequestError(msg, error.response))
        }

        const msg = extractServerMessage(data, t('http.requestFailed'))
        const bizCode = data && data.code

        switch (status) {
          case 401:
            if (!silent) showToast(t('http.loginExpired'))
            clearAuthAndGoLogin()
            break
          case 403:
            if (!silent) showToast(t('http.accessDenied'))
            break
          case 404:
            if (!silent) showToast(t('http.notFound'))
            break
          case 429:
            if (!silent) showToast(msg || t('http.tooManyRequests'))
            break
          case 500:
          case 502:
          case 503:
          case 504:
            if (!silent) showToast(t('http.serverError'))
            break
          default:
            if (!silent) showToast(msg)
        }
        return Promise.reject(buildRequestError(msg, error.response, bizCode))
      }

      if (error.request) {
        // 请求已发出但没有收到响应（超时/服务器无响应）
        if (!error.config || !error.config._silent) showToast(t('http.networkTimeout'))
        return Promise.reject(error)
      }
      // 请求配置出错
      if (!error.config || !error.config._silent) showToast(t('http.configError'))
      return Promise.reject(error)
    }
  )
}

// 初始化请求实例
let request = createRequestInstance()
addInterceptors()

// 供 http.js 获取当前 request 实例（支持 updateRequestConfig 重新赋值后仍能获取最新实例）
export function getRequestInstance() {
  return request
}

// 更新请求实例配置
export const updateRequestConfig = async () => {
  try {
  const apiConfig = getApiConfig()
  // 默认走相对路径（同源，由 vite 代理转发到后端：dev=8204、docker=8204）；
  // 环境变量 VITE_API_BASE_URL 可配置为绝对地址（跨域/远程部署），
  // 只有当 localStorage 显式存了 baseUrl 时才覆盖为绝对地址。
  const apiBaseUrl = apiConfig.baseUrl || import.meta.env?.VITE_API_BASE_URL || ''
    
    // 避免重复创建实例导致的问题
    if (request && request.defaults && request.defaults.baseURL === apiBaseUrl) {
      return { baseURL: apiBaseUrl }
    }
    
    request = axios.create({
      baseURL: apiBaseUrl,
      headers: {
        'Content-Type': 'application/json'
      }
    })
    addInterceptors()
    return { baseURL: apiBaseUrl }
  } catch (error) {
    console.error('更新请求配置失败:', error)
    throw error
  }
}

// 从 http.js 重导出 http 对象（向后兼容）
export { http } from './http'

export default request
