import request from '@/utils/request'

// AI 内容创作 - 匹配后端 /api/ai/* 路径
export function generateAIContent(data) {
  return request({ url: '/api/ai/generate', method: 'post', data })
}
export function getGenerationHistory(params) {
  return request({ url: '/api/ai/history', method: 'get', params })
}
export function getAIContentRecord(id) {
  return request({ url: `/api/ai/history/${id}`, method: 'get' })
}
export function saveAIContentRecord(id) {
  return request({ url: `/api/ai/history/${id}/save`, method: 'post' })
}
export function favoriteAIContentRecord(id) {
  return request({ url: `/api/ai/history/${id}/favorite`, method: 'post' })
}
export function rateAIContentRecord(id, rating) {
  return request({ url: `/api/ai/history/${id}/rate`, method: 'post', data: { rating } })
}
export function deleteAIContentRecord(id) {
  return request({ url: `/api/ai/history/${id}`, method: 'delete' })
}
export function getAITemplates() {
  return request({ url: '/api/ai/templates', method: 'get' })
}
export function getAITemplate(id) {
  return request({ url: `/api/ai/templates/${id}`, method: 'get' })
}
export function createAITemplate(data) {
  return request({ url: '/api/ai/templates', method: 'post', data })
}
export function updateAITemplate(id, data) {
  return request({ url: `/api/ai/templates/${id}`, method: 'put', data })
}
export function deleteAITemplate(id) {
  return request({ url: `/api/ai/templates/${id}`, method: 'delete' })
}
export function getAITemplateTypes() {
  return request({ url: '/api/ai/template-types', method: 'get' })
}

// 兼容旧接口
export function getAIHistory(params) {
  return getGenerationHistory(params)
}
export function deleteAIHistory(id) {
  return deleteAIContentRecord(id)
}
