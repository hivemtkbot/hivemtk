import { http } from '@/utils/http'

// Entity 实体图 / JSON-LD 生成
export const listEntities = (type, keyword) =>
  http.get('/api/geo/entities', { type: type || '', keyword: keyword || '' })

export const getEntityRelations = (entityId) =>
  http.get(`/api/geo/entities/${entityId}/relations`)

export const extractEntities = (docId) =>
  http.post('/api/geo/entities/extract', { doc_id: docId })

export const generateEntitySchema = (entityId) =>
  http.get('/api/geo/entities/schema', { entity_id: entityId })

// 知识库 / 文档管理（GEO 侧）
export const listKBDocuments = (keyword, sourceLevel) =>
  http.get('/api/geo/kb/documents', { keyword: keyword || '', source_level: sourceLevel || '' })

export const uploadKBDocument = (formData) =>
  http.upload('/api/geo/kb/upload', formData)

export const deleteKBDocument = (id) =>
  http.delete(`/api/geo/kb/documents/${id}`)

export const updateKBDocument = (id, data) =>
  http.post(`/api/geo/kb/documents/${id}`, data)

export const askKB = (question) =>
  http.post('/api/geo/kb/ask', { question })

// 工作流相关
export const listWorkflows = () =>
  http.get('/api/geo/workflows')

export const saveWorkflow = (data) =>
  http.post('/api/geo/workflows', data)

export const deleteWorkflow = (id) =>
  http.delete(`/api/geo/workflows/${id}`)

export const runWorkflow = (id, params) =>
  http.post(`/api/geo/workflows/${id}/run`, params || {})

export const listWorkflowExecutions = (workflowId) =>
  http.get('/api/geo/workflows/executions', { workflow_id: workflowId || '' })
