import { http } from '@/utils/request';

// 异议处理 - 匹配后端 objection_handler_controller
// 路由: /api/objection/*

// 智能处理异议：分类 + 匹配模板 + 生成建议
export function handleObjection(data) {
  return http.post('/api/objection/handle', data)
}

// 仅分类（不匹配模板）
export function classifyObjection(data) {
  return http.post('/api/objection/classify', data)
}

// 列出所有异议类别
export function listObjectionCategories() {
  return http.get('/api/objection/categories')
}

// 记录异议模板使用结果
export function recordObjectionUsage(data) {
  return http.post('/api/objection/usage', data)
}
