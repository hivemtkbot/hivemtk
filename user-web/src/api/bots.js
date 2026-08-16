import { http } from '@/utils/request'

/**
 * 群机器人通用化（USR-SM-02）
 * 借鉴：Microsoft Bot Framework
 * Telegram / 飞书 / 钉钉 webhook 统一抽象
 */

// 注册渠道
export const registerBot = (data) => http.post('/api/bots/register', data)
export const listBots = (params) => http.get('/api/bots', params)
export const getBot = (id) => http.get(`/api/bots/${id}`)
export const updateBot = (id, data) => http.put(`/api/bots/${id}`, data)
export const deleteBot = (id) => http.delete(`/api/bots/${id}`)

// 接收群消息（统一 webhook）
export const handleBotWebhook = (channel, data) =>
  http.post(`/api/bots/${channel}/webhook`, data)

// 群消息 → CRM 事件
export const syncBotToCRM = (botId) => http.post(`/api/bots/${botId}/sync-crm`, {})

// 群内 AI 智能体挂载
export const mountAgentToBot = (botId, agentId) =>
  http.post(`/api/bots/${botId}/mount-agent`, { agent_id: agentId })
