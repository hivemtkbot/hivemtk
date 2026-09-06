import { getRequestInstance } from './request';

export const http = {
  get(url, params, config = {}) {
    let realParams = params
    if (realParams &&
    typeof realParams === 'object' &&
    !Array.isArray(realParams) &&
    Object.keys(realParams).length === 1 &&
    'params' in realParams) {
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