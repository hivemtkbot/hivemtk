import { http } from '@/utils/http'

// ===== Entity 实体图谱（后端 /geo/entity/*） =====
export const listEntities = (type, keyword) =>
  http.get('/api/geo/entity/list', { type: type || '', keyword: keyword || '' })

export const getEntityRelations = (entityId) =>
  http.get(`/api/geo/entity/${entityId}/graph`)

export const extractEntities = (docId) =>
  http.post('/api/geo/entities/extract', { doc_id: docId })

// ===== 知识库 / 文档管理（后端 /geo/kb/*） =====
export const listKBDocuments = (keyword, sourceLevel) =>
  http.get('/api/geo/kb/documents', { keyword: keyword || '', source_level: sourceLevel || '' })

export const saveKBDocument = (data) =>
  http.post('/api/geo/kb/documents', data)

export const deleteKBDocument = (id) =>
  http.delete(`/api/geo/kb/documents/${id}`)

export const getKBDocument = (id) =>
  http.get(`/api/geo/kb/documents/${id}`)

export const searchKB = (q, limit = 10) =>
  http.get('/api/geo/kb/search', { q, limit })

export const askKB = (question) =>
  http.post('/api/geo/kb/ask', { question })

// ===== 工作流（后端 /geo/workflow/workflows*） =====
export const listWorkflows = () =>
  http.get('/api/geo/workflow/workflows')

export const getWorkflow = (id) =>
  http.get(`/api/geo/workflow/workflows/${id}`)

export const saveWorkflow = (data) =>
  http.post('/api/geo/workflow/workflows', data)

export const updateWorkflow = (id, data) =>
  http.put(`/api/geo/workflow/workflows/${id}`, data)

export const deleteWorkflow = (id) =>
  http.delete(`/api/geo/workflow/workflows/${id}`)

export const runWorkflow = (id, params) =>
  http.post(`/api/geo/workflow/workflows/${id}/run`, params || {})

export const listWorkflowExecutions = (workflowId) =>
  http.get(`/api/geo/workflow/workflows/${workflowId}/executions`)

// ===== 工作流模板 =====
export const listWorkflowTemplates = () =>
  http.get('/api/geo/workflow/templates')

export const saveWorkflowTemplate = (data) =>
  http.post('/api/geo/workflow/templates', data)
