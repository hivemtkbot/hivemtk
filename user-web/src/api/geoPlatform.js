import { http } from '@/utils/http'

// 平台发布 Pipeline 相关
export const testPlatformPublish = (platform, content) =>
  http.post('/api/geo/platform/pipeline/test', { platform, content })

export const runPlatformPipeline = (articleId, platforms) =>
  http.post('/api/geo/platform/pipeline/run', { article_id: articleId, platforms })

export const getPipelineStatus = (articleId) =>
  http.get(`/api/geo/platform/pipeline/status/${articleId}`)

export const listPlatformAccounts = () =>
  http.get('/api/geo/platform/accounts')

export const savePlatformAccount = (data) =>
  http.post('/api/geo/platform/accounts/save', data)

export const listPlatformPublishRecords = (articleId) =>
  http.get('/api/geo/platform/records', { article_id: articleId })
