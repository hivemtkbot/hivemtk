import { http } from '@/utils/request';

// 客户旅程大屏 - 匹配后端 customer_journey_controller
// 路由: /api/customer-journey/*

// 获取总览（所有阶段客户数 + 转化率）
export function getJourneyOverview() {
  return http.get('/api/customer-journey/overview')
}

// 获取单个客户旅程状态
export function getJourneyState(customerId) {
  return http.get(`/api/customer-journey/overview?customer_id=${customerId}`)
}

// 列出所有阶段配置
export function listJourneyStages() {
  return http.get('/api/customer-journey/stages')
}

// 按阶段列出客户
export function listByStage(stage) {
  return http.get(`/api/customer-journey/by-stage?stage=${stage}`)
}

// 迁移客户阶段
export function transitionJourney(data) {
  return http.post('/api/customer-journey/transition', data)
}

// 记录客户互动
export function touchCustomer(data) {
  return http.post('/api/customer-journey/touch', data)
}
