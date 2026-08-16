import { http } from '@/utils/request'

/**
 * 协作功能（USR-WB-06）
 * - @mention 通知
 * - 内部 note（不发给访客）
 * - 多人协作碰撞检测
 */

// 发送消息（可选 isInternalNote）
export const sendCollaborativeMessage = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/messages`, data)

// 标记为内部 note
export const addInternalNote = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/internal-notes`, data)

// @mention 用户搜索
export const searchMentionUsers = (params) =>
  http.get('/api/users/search', params)

// 标记消息为 @mention 已读
export const markMentionRead = (mentionId) =>
  http.post(`/api/mentions/${mentionId}/read`, {})

// 获取我的 @mention 列表
export const getMyMentions = (params) =>
  http.get('/api/mentions/mine', params)

// 协作锁：声明正在编辑（防碰撞）
export const acquireEditLock = (sessionId) =>
  http.post(`/api/customer-sessions/${sessionId}/edit-lock`, { holder: 'me' }, { _silent: true })

// 释放协作锁
export const releaseEditLock = (sessionId) =>
  http.delete(`/api/customer-sessions/${sessionId}/edit-lock`, { _silent: true })

// 查询协作锁状态
export const getEditLock = (sessionId) =>
  http.get(`/api/customer-sessions/${sessionId}/edit-lock`, { _silent: true })

// 查询会话内部 note
export const listInternalNotes = (sessionId, params) =>
  http.get(`/api/customer-sessions/${sessionId}/internal-notes`, params)
