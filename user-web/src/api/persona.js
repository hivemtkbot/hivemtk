import { http } from '@/utils/request';

export function listStaffs() {
  return http.get('/api/analytics/persona/staffs')
}

export function getPersonaReport(staffId) {
  return http.get(`/api/analytics/persona/staffs/${staffId}`)
}
