import { http } from '@/utils/request';

export function getJourneyOverview() {
  return http.get('/api/customer-journey/overview')
}

export function getJourneyState(customerId) {
  return http.get(`/api/customer-journey/overview?customer_id=${customerId}`)
}

export function listJourneyStages() {
  return http.get('/api/customer-journey/stages')
}

export function listByStage(stage) {
  return http.get(`/api/customer-journey/by-stage?stage=${stage}`)
}

export function transitionJourney(data) {
  return http.post('/api/customer-journey/transition', data)
}

export function touchCustomer(data) {
  return http.post('/api/customer-journey/touch', data)
}
