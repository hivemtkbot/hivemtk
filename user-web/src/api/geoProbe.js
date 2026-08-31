import { http } from '@/utils/http'

// 探针 / SOV / 爬虫统计相关
export const testEngineProbe = (engine, query) =>
  http.post('/api/geo/probe/test', { engine, query })

export const listProbeRuns = (engine, limit = 100) =>
  http.get('/api/geo/probe/runs', { engine, limit })

export const getSOVTrend = (intent, funnelStage, days = 30) =>
  http.get('/api/geo/sov/trend', {
    intent,
    funnel_stage: funnelStage,
    days
  })

export const listDailyStats = (engine, dateFrom, dateTo) =>
  http.get('/api/geo/daily-stats', {
    engine,
    date_from: dateFrom,
    date_to: dateTo
  })

export const getSourceCatalog = () =>
  http.get('/api/geo/source-catalog')

export const upsertSourceCatalog = (data) =>
  http.post('/api/geo/source-catalog/upsert', data)

export const runSourceCatalogSync = () =>
  http.post('/api/geo/source-catalog/sync')
