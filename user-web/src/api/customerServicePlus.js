import { http } from '@/utils/request'

/**
 * 客服子功能 4 件套强化（USR-SM-03）
 * 快捷回复 / AI 建议 / 坐席状态 / 会话标签 增强
 */

// 快捷回复（文件夹分类 + 拖拽排序）
export const listQuickReplyFolders = () => http.get('/api/quick-reply/folders')
export const createQuickReplyFolder = (data) => http.post('/api/quick-reply/folders', data)
export const reorderQuickReplies = (folderId, orderData) =>
  http.post(`/api/quick-reply/folders/${folderId}/reorder`, orderData)

// AI 建议（带置信度 + 来源）
export const getAISuggestionsWithScore = (sessionId) =>
  http.get(`/api/customer-service/ai-suggestions?session_id=${sessionId}&with_score=true`)

// 会话标签（规则自动打标）
export const listTagRules = () => http.get('/api/session-tag/rules')
export const createTagRule = (data) => http.post('/api/session-tag/rules', data)
export const applyTagRule = (sessionId) => http.post(`/api/customer-sessions/${sessionId}/apply-tag-rule`, {})

// 坐席状态（详细看板）
export const getAgentStatusBoard = () => http.get('/api/customer-service/agent-status-board')
