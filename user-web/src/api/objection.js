import request from '@/utils/request'

// 异议处理 - 匹配后端 objection_handler_controller
// 路由: /api/objection/*

// 智能处理异议：分类 + 匹配模板 + 生成建议
export function handleObjection(data) {
  return request({ url: '/api/objection/handle', method: 'post', data })
}

// 仅分类（不匹配模板）
export function classifyObjection(data) {
  return request({ url: '/api/objection/classify', method: 'post', data })
}

// 列出所有异议类别
export function listObjectionCategories() {
  return request({ url: '/api/objection/categories', method: 'get' })
}

// 记录异议模板使用结果
export function recordObjectionUsage(data) {
  return request({ url: '/api/objection/usage', method: 'post', data })
}
