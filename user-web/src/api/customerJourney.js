import request from '@/utils/request'

// 客户旅程大屏 - 匹配后端 customer_journey_controller
// 路由: /api/customer-journey/*

// 获取总览（所有阶段客户数 + 转化率）
export function getJourneyOverview() {
  return request({ url: '/api/customer-journey/overview', method: 'get' })
}

// 获取单个客户旅程状态
export function getJourneyState(customerId) {
  return request({ url: `/api/customer-journey/overview?customer_id=${customerId}`, method: 'get' })
}

// 列出所有阶段配置
export function listJourneyStages() {
  return request({ url: '/api/customer-journey/stages', method: 'get' })
}

// 按阶段列出客户
export function listByStage(stage) {
  return request({ url: `/api/customer-journey/by-stage?stage=${stage}`, method: 'get' })
}

// 迁移客户阶段
export function transitionJourney(data) {
  return request({ url: '/api/customer-journey/transition', method: 'post', data })
}

// 记录客户互动
export function touchCustomer(data) {
  return request({ url: '/api/customer-journey/touch', method: 'post', data })
}
