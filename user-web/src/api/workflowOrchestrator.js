import { http } from '@/utils/request'

// 工作流编排 API - 匹配后端 /api/workflows/* 路径

export const workflowOrchestratorApi = {
  // === 版本管理 ===
  // 列出版本：
  //   - 传 string：按 workflow_id 不分页查询（兼容 Editor.vue 旧用法）
  //   - 传 object：服务端分页 { workflow_id?, status?, page?, page_size? }，返回 { list, total, page, page_size }
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

  // 获取版本详情
  getVersion(id) {
    return http.get(`/api/workflows/versions/${id}`)
  },

  // 创建版本
  createVersion(data) {
    return http.post('/api/workflows/versions', data)
  },

  // 更新版本
  updateVersion(id, data) {
    return http.put(`/api/workflows/versions/${id}`, data)
  },

  // 删除版本
  deleteVersion(id) {
    return http.delete(`/api/workflows/versions/${id}`)
  },

  // 发布版本
  publishVersion(id) {
    return http.post(`/api/workflows/versions/${id}/publish`)
  },

  // 归档版本
  archiveVersion(id) {
    return http.post(`/api/workflows/versions/${id}/archive`)
  },

  // === 执行管理 ===
  // 执行工作流
  execute(data) {
    return http.post('/api/workflows/execute', data)
  },

  // 列出执行实例
  listExecutions(params) {
    return http.get('/api/workflows/executions', params)
  },

  // 获取执行详情
  getExecution(id) {
    return http.get(`/api/workflows/executions/${id}`)
  },

  // 获取节点执行明细
  getNodeExecutions(executionId) {
    return http.get(`/api/workflows/executions/${executionId}/nodes`)
  },

  // 停止执行
  stopExecution(id) {
    return http.post(`/api/workflows/executions/${id}/stop`)
  }
}

export default workflowOrchestratorApi
