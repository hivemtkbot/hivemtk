import { http } from '@/utils/request'

export const geoApi = {
  mineKeywords(data) {
    return http.post('/api/geo/keywords/mine', data)
  },

  semanticExpand(data) {
    return http.post('/api/geo/keywords/expand', data)
  },

  topicCluster(data) {
    return http.post('/api/geo/keywords/cluster', data)
  },

  getKeywordList(params) {
    return http.get('/api/geo/keywords/list', params)
  },

  deleteKeyword(id) {
    return http.delete(`/api/geo/keywords/${id}`)
  },

  generateContent(data) {
    return http.post('/api/geo/content/generate', data)
  },

  scoreContent(data) {
    return http.post('/api/geo/content/score', data)
  },

  optimizeContent(data) {
    return http.post('/api/geo/content/optimize', data)
  },

  enhanceEEAT(data) {
    return http.post('/api/geo/content/eeat', data)
  },

  generateSchema(data) {
    return http.post('/api/geo/content/schema', data)
  },

  checkUniqueness(data) {
    return http.post('/api/geo/content/uniqueness', data)
  },

  getArticleList(params) {
    return http.get('/api/geo/content/list', params)
  },

  getArticleByID(id) {
    return http.get(`/api/geo/content/${id}`)
  },

  verifyArticle(data) {
    return http.post('/api/geo/verification/verify', data)
  },

  monitorNegative(data) {
    return http.post('/api/geo/verification/negative', data)
  },

  getVerifyResults(articleId) {
    return http.get(`/api/geo/verification/results/${articleId}`)
  },

  getReport(params) {
    return http.get('/api/geo/reports/summary', params)
  },

  getROI(params) {
    return http.get('/api/geo/reports/roi', params)
  },

  getAPICosts(params) {
    return http.get('/api/geo/reports/api-costs', params)
  },

  getConfig() {
    return http.get('/api/geo/config')
  },

  updateConfig(data) {
    return http.put('/api/geo/config', data)
  },

  optimizeConfig(data) {
    return http.post('/api/geo/config/optimize', data)
  }
};

export default geoApi
