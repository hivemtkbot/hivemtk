import { http } from '@/utils/request';

export function getLicenseStatus() {
  return http.get('/api/license/status')
}