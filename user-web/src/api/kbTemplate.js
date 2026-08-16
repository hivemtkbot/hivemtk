import { http } from '@/utils/request'

/**
 * 知识库模板市场订阅（USR-KB-05）
 */

export const listTemplates = (params) => http.get('/api/kb-templates', params)
export const getTemplate = (id) => http.get(`/api/kb-templates/${id}`)
export const subscribeTemplate = (id) => http.post(`/api/kb-templates/${id}/subscribe`, {})
export const applyTemplate = (id, kbId) => http.post(`/api/kb-templates/${id}/apply`, { kb_id: kbId })
export const rateTemplate = (id, data) => http.post(`/api/kb-templates/${id}/rate`, data)
export const listSubscribed = () => http.get('/api/kb-templates/subscribed')
export const unsubscribeTemplate = (id) => http.delete(`/api/kb-templates/${id}/subscribe`)
