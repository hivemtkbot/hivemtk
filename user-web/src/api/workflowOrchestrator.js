import { http } from '@/utils/request'

export const workflowOrchestratorApi = {
  listVersions(workflowIdOrParams) {
    if (workflowIdOrParams === null || workflowIdOrParams === undefined) {
      return http.get('/api/workflows/versions', {})
    }
    if (typeof workflowIdOrParams === 'string') {
      return http.get('/api/workflows/versions', { workflow_id: workflowIdOrParams })
    }
    const params = {
      workflow_id: workflowIdOrParams.workflow_id || '',
      status: workflowIdOrParams.status || '',
      page: workflowIdOrParams.page || 1,
      page_size: workflowIdOrParams.page_size || workflowIdOrParams.pageSize || 20
    }
    return http.get('/api/workflows/versions', params)
  },

  getVersion(id) {
    return http.get(`/api/workflows/versions/${id}`)
  },

  createVersion(data) {
    return http.post('/api/workflows/versions', data)
  },

  updateVersion(id, data) {
    return http.put(`/api/workflows/versions/${id}`, data)
  },

  deleteVersion(id) {
    return http.delete(`/api/workflows/versions/${id}`)
  },

  publishVersion(id) {
    return http.post(`/api/workflows/versions/${id}/publish`)
  },

  archiveVersion(id) {
    return http.post(`/api/workflows/versions/${id}/archive`)
  },

  execute(data) {
    return http.post('/api/workflows/execute', data)
  },

  listExecutions(params) {
    return http.get('/api/workflows/executions', params)
  },

  getExecution(id) {
    return http.get(`/api/workflows/executions/${id}`)
  },

  getNodeExecutions(executionId) {
    return http.get(`/api/workflows/executions/${executionId}/nodes`)
  },

  stopExecution(id) {
    return http.post(`/api/workflows/executions/${id}/stop`)
  }
};

export default workflowOrchestratorApi
