import { http } from '@/utils/request'

export const listQuickReplyFolders = () => http.get('/api/quick-reply/folders');
export const createQuickReplyFolder = (data) => http.post('/api/quick-reply/folders', data)
export const reorderQuickReplies = (folderId, orderData) =>
  http.post(`/api/quick-reply/folders/${folderId}/reorder`, orderData)

export const getAISuggestionsWithScore = (sessionId) =>
  http.get(`/api/customer-service/ai-suggestions?session_id=${sessionId}&with_score=true`);

export const listTagRules = () => http.get('/api/session-tag/rules');
export const createTagRule = (data) => http.post('/api/session-tag/rules', data)
export const applyTagRule = (sessionId) => http.post(`/api/customer-sessions/${sessionId}/apply-tag-rule`, {})

export const getAgentStatusBoard = () => http.get('/api/customer-service/agent-status-board');
