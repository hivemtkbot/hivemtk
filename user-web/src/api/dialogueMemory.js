import { http } from '@/utils/request'

export const memoryApi = {
  appendMessage(data) {
    return http.post('/api/memory/messages', data)
  },

  getShortTerm(params) {
    return http.get('/api/memory/short', params)
  },

  getLongTerm(params) {
    return http.get('/api/memory/long', params)
  },

  updateKeyFacts(data) {
    return http.post('/api/memory/facts', data)
  },

  recordObjection(data) {
    return http.post('/api/memory/objections', data)
  },

  updatePurchaseIntent(data) {
    return http.post('/api/memory/purchase-intent', data)
  },

  recordIntent(data) {
    return http.post('/api/memory/intent-trail', data)
  },

  recordSOP(data) {
    return http.post('/api/memory/sop-history', data)
  },

  buildContext(params) {
    return http.get('/api/memory/context', params)
  },

  list(params) {
    return http.get('/api/memory/list', params)
  }
};
