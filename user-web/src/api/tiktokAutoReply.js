import request from '@/utils/request'

// TikTok自动回复相关API

// 获取TikTok自动回复账号列表
export function getTikTokAccounts() {
  return request({
      url: '/api/tiktok/auto-reply/accounts',
      method: 'get'
    })
}

// 获取TikTok自动回复规则
export function getTikTokRule() {
  return request({
      url: '/api/tiktok/auto-reply/rule',
      method: 'get'
    })
}

// 保存TikTok自动回复规则
export function saveTikTokRule(data) {
  return request({
      url: '/api/tiktok/auto-reply/rule',
      method: 'post',
      data
    })
}

// 更新或创建TikTok账号
export function upsertTikTokAccount(data) {
  return request({
      url: '/api/tiktok/auto-reply/accounts',
      method: 'post',
      data
    })
}

// 删除TikTok账号
export function deleteTikTokAccount(accountId) {
  return request({
      url: `/api/tiktok/auto-reply/accounts/${accountId}`,
      method: 'delete'
    })
}

// 获取TikTok自动回复日志
export function getTikTokLogs(params) {
  return request({
      url: '/api/tiktok/auto-reply/logs',
      method: 'get',
      params
    })
}

// 启动TikTok自动回复
export function startTikTokAutoReply(data) {
  return request({
      url: '/api/tiktok/auto-reply/start',
      method: 'post',
      data
    })
}

// 停止TikTok自动回复
export function stopTikTokAutoReply(data) {
  return request({
      url: '/api/tiktok/auto-reply/stop',
      method: 'post',
      data
    })
}