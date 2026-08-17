import { http } from '@/utils/request';

// ============================================================================
// AI 工具配置 API
// ============================================================================

// 获取工具列表
export function listTools(params) {
  return http.get('/api/ai-tools', params)
}

// 获取工具详情
export function getTool(name) {
  return http.get(`/api/ai-tools/${name}`)
}

// 更新工具启用状态
export function updateToolStatus(name, enabled) {
  return http.put(`/api/ai-tools/${name}/status`, { is_enabled: enabled })
}

// 批量更新工具状态
export function batchUpdateToolStatus(tools, enabled) {
  return http.post('/api/ai-tools/batch-status', { tools, is_enabled: enabled })
}

// ============================================================================
// 工具-账号绑定 API
// ============================================================================

// 获取工具绑定的账号
export function getToolAccounts(toolName) {
  return http.get(`/api/ai-tools/${toolName}/accounts`)
}

// 绑定账号到工具
export function bindToolAccount(toolName, accountType, accountId, isPrimary) {
  return http.post(`/api/ai-tools/${toolName}/accounts`, {
    account_type: accountType,
    account_id: accountId,
    is_primary: isPrimary
  })
}

// 解绑账号
export function unbindToolAccount(toolName, accountType, accountId) {
  return http.delete(`/api/ai-tools/${toolName}/accounts/${accountType}/${accountId}`)
}