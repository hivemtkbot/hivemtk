import { http } from '@/utils/request';

export function listTools(params) {
  return http.get('/api/ai-tools', params)
}

export function getTool(name) {
  return http.get(`/api/ai-tools/${name}`)
}

export function updateToolStatus(name, enabled) {
  return http.put(`/api/ai-tools/${name}/status`, { is_enabled: enabled })
}

export function batchUpdateToolStatus(tools, enabled) {
  return http.post('/api/ai-tools/batch-status', { tools, is_enabled: enabled })
}

export function getToolAccounts(toolName) {
  return http.get(`/api/ai-tools/${toolName}/accounts`)
}

export function bindToolAccount(toolName, accountType, accountId, isPrimary) {
  return http.post(`/api/ai-tools/${toolName}/accounts`, {
    account_type: accountType,
    account_id: accountId,
    is_primary: isPrimary
  })
}

export function unbindToolAccount(toolName, accountType, accountId) {
  return http.delete(`/api/ai-tools/${toolName}/accounts/${accountType}/${accountId}`)
}