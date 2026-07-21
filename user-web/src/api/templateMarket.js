import request from '@/utils/request'

// 模板市场 - 匹配后端 /api/templates/* 路径
export function getTemplateMarketList(params) {
  return request({ url: '/api/templates', method: 'get', params })
}
export function getTemplateMarketDetail(id) {
  return request({ url: `/api/templates/${id}`, method: 'get' })
}
export function downloadTemplate(id) {
  return request({ url: `/api/templates/${id}/download`, method: 'post' })
}
export function getOfficialTemplates() {
  return request({ url: '/api/templates/official', method: 'get' })
}
export function searchTemplates(params) {
  return request({ url: '/api/templates/search', method: 'get', params })
}
export function getMyDownloads() {
  return request({ url: '/api/templates/my-downloads', method: 'get' })
}

// 兼容旧接口
export function getTemplates(params) {
  return getTemplateMarketList(params)
}
export function submitTemplate(data) {
  return request({ url: '/api/templates', method: 'post', data })
}
export function useTemplate(id) {
  return downloadTemplate(id)
}
export function rateTemplate(id, rating) {
  return request({ url: `/api/templates/${id}/rate`, method: 'post', data: { rating } })
}
export function getMyTemplates() {
  return getMyDownloads()
}
