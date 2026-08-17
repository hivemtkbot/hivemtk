import { http } from '@/utils/request';

// 备份列表
export function getBackupList(params) {
  return http.get('/api/backups', params)
}

// 备份详情
export function getBackupByID(id) {
  return http.get(`/api/backups/${id}`)
}

// 创建备份
export function createBackup(data) {
  return http.post('/api/backups', data)
}

// 删除备份
export function deleteBackup(id) {
  return http.delete(`/api/backups/${id}`)
}

// 触发恢复
export function restoreBackup(data) {
  return http.post('/api/restore', data)
}

// 恢复记录列表
export function getRestoreList(params) {
  return http.get('/api/restore/list', params)
}

// 最近一次恢复
export function getLastRestore() {
  return http.get('/api/restore/last')
}
