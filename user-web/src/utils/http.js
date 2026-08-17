/**
 * 共享 HTTP 请求封装
 *
 * 导出语义化的 HTTP 方法对象，新代码统一使用：
 *   import { http } from '@/utils/http'
 *   http.get(url, params) / http.post(url, data) / http.put / http.delete / http.upload
 *
 * 底层依赖 request.js 创建的 axios 实例，通过 getRequestInstance() 获取，
 * 确保 updateRequestConfig 重新赋值后各方法仍能使用最新实例。
 */
import { getRequestInstance } from './request'

export const http = {
  get(url, params, config = {}) {
    let realParams = params
    // 兜底：部分调用方误写成 http.get(url, { params })，导致参数被二次包裹
    if (
      realParams &&
      typeof realParams === 'object' &&
      !Array.isArray(realParams) &&
      Object.keys(realParams).length === 1 &&
      'params' in realParams
    ) {
      realParams = realParams.params
    }
    return getRequestInstance().get(url, { params: realParams, ...config })
  },

  post(url, data, config = {}) {
    return getRequestInstance().post(url, data, config)
  },

  put(url, data, config = {}) {
    return getRequestInstance().put(url, data, config)
  },

  delete(url, params, config = {}) {
    let realParams = params
    if (
      realParams &&
      typeof realParams === 'object' &&
      !Array.isArray(realParams) &&
      Object.keys(realParams).length === 1 &&
      'params' in realParams
    ) {
      realParams = realParams.params
    }
    return getRequestInstance().delete(url, { params: realParams, ...config })
  },

  upload(url, formData, config = {}) {
    return getRequestInstance().post(url, formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      },
      ...config
    })
  }
}