import { http } from '@/utils/request';

export function getBackupList(params) {
  return http.get('/api/backups', params)
}

export function getBackupByID(id) {
  return http.get(`/api/backups/${id}`)
}

export function createBackup(data) {
  return http.post('/api/backups', data)
}

export function deleteBackup(id) {
  return http.delete(`/api/backups/${id}`)
}

export function restoreBackup(data) {
  return http.post('/api/restore', data)
}

export function getRestoreList(params) {
  return http.get('/api/restore/list', params)
}

export function getLastRestore() {
  return http.get('/api/restore/last')
}
