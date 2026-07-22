import request from '@/utils/request'

// 市场
export const listAssets = (params) =>
  request({ url: '/api/v1/asset-market/list', method: 'get', params })

export const assetDetail = (id) =>
  request({ url: `/api/v1/asset-market/detail/${id}`, method: 'get' })

export const purchaseAsset = (data) =>
  request({ url: '/api/v1/asset-market/purchase', method: 'post', data })

export const syncAsset = (data) =>
  request({ url: '/api/v1/asset-market/sync', method: 'post', data })

// 本地资产（同源同构）
export const listLocalAssets = (params) =>
  request({ url: '/api/v1/local-assets', method: 'get', params })

export const getLocalAsset = (id) =>
  request({ url: `/api/v1/local-assets/${id}`, method: 'get' })

export const createLocalAsset = (data) =>
  request({ url: '/api/v1/local-assets', method: 'post', data })

export const updateLocalAsset = (id, data) =>
  request({ url: `/api/v1/local-assets/${id}`, method: 'put', data })

export const deleteLocalAsset = (id) =>
  request({ url: `/api/v1/local-assets/${id}`, method: 'delete' })

export const toggleLocalAsset = (id, active) =>
  request({ url: `/api/v1/local-assets/${id}/toggle-active`, method: 'put', data: { active } })

export const syncLog = (params) =>
  request({ url: '/api/v1/local-assets/sync-log', method: 'get', params })
