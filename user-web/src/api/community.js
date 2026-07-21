import { http } from '@/utils/request'

// 社群管理 API 接口
export const communityApi = {
  // 获取社群分组列表
  getGroups(params) {
    return http.get('/api/community/groups', { params })
  },

  // 创建社群分组
  createGroup(data) {
    return http.post('/api/community/groups', data)
  },

  // 获取社群分组详情
  getGroupById(id) {
    return http.get(`/api/community/groups/${id}`)
  },

  // 更新社群分组
  updateGroup(id, data) {
    return http.put(`/api/community/groups/${id}`, data)
  },

  // 删除社群分组
  deleteGroup(id) {
    return http.delete(`/api/community/groups/${id}`)
  },

  // 获取社群成员列表
  getMembers(params) {
    return http.get('/api/community/members', { params })
  },

  // 获取社群消息列表
  getMessages(params) {
    return http.get('/api/community/messages', { params })
  },

  // 获取社群统计
  getStats(params) {
    return http.get('/api/community/stats', { params })
  },

  // 导入社群数据
  importData(data) {
    return http.post('/api/community/import', data)
  },

  // 导出社群数据
  exportData(data) {
    return http.post('/api/community/export', data)
  }
}
