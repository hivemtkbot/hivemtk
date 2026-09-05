import { http } from '@/utils/request';

export function getLeadMiningConfig() {
  return http.get('/api/lead-mining/config')
}

export function saveLeadMiningConfig(data) {
  return http.post('/api/lead-mining/config', data)
}
