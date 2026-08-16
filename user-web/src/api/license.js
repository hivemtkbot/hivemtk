import { http } from '@/utils/request';

/**
 * 获取授权状态信息
 */
export function getLicenseStatus() {
  return http.get('/api/license/status')
}