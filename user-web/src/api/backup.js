import request from '@/utils/request'

// 备份列表
export function getBackupList(params) {
  return request({ url: '/api/backups', method: 'get', params })
}

// 备份详情
export function getBackupByID(id) {
  return request({ url: `/api/backups/${id}`, method: 'get' })
}

// 创建备份
export function createBackup(data) {
  return request({ url: '/api/backups', method: 'post', data })
}

// 删除备份
export function deleteBackup(id) {
  return request({ url: `/api/backups/${id}`, method: 'delete' })
}

// 触发恢复
export function restoreBackup(data) {
  return request({ url: '/api/restore', method: 'post', data })
}

// 恢复记录列表
export function getRestoreList(params) {
  return request({ url: '/api/restore/list', method: 'get', params })
}

// 最近一次恢复
export function getLastRestore() {
  return request({ url: '/api/restore/last', method: 'get' })
}
