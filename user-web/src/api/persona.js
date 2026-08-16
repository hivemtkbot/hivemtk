import { http } from '@/utils/request';

// 销冠能力画像 - 匹配后端 sales_persona_controller
// 路由: /api/analytics/persona/*

// 列出所有员工
export function listStaffs() {
  return http.get('/api/analytics/persona/staffs')
}

// 获取指定员工的能力画像报告
export function getPersonaReport(staffId) {
  return http.get(`/api/analytics/persona/staffs/${staffId}`)
}
