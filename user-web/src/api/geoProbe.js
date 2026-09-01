import { http } from '@/utils/http'

// ===== 探针（后端 /geo/probe/*） =====
export const listProbeEngines = () =>
  http.get('/api/geo/probe/engines')

export const testEngineProbe = (engine, query) =>
  http.post('/api/geo/probe/test', { engine, query })

export const listProbeRuns = (engine, limit = 100) =>
  http.get('/api/geo/probe/runs', { engine, limit })

export const runSOVRefresh = () =>
  http.post('/api/geo/probe/run-sov')

export const runNegativeMonitor = () =>
  http.post('/api/geo/probe/run-negative')

export const runSourceCatalogSync = () =>
  http.post('/api/geo/probe/run-source-sync')

export const probeAllEngines = (engines, query) =>
  http.post('/api/geo/probe/all', { engines, query })

// ===== SOV / 爬虫 / 不准确（后端 /geo/sov, /geo/crawler-stats, /geo/inaccurate-claims） =====
export const getSOV = () =>
  http.get('/api/geo/sov')

export const getCrawlerStats = () =>
  http.get('/api/geo/crawler-stats')

export const runCrawler = () =>
  http.post('/api/geo/crawler/run')

export const detectInaccurateClaims = (brandName) =>
  http.post('/api/geo/inaccurate-claims', { brand_name: brandName })

// ===== 信源目录（后端 /geo/source-catalog/levels） =====
export const lookupSourceLevel = (url) =>
  http.get('/api/geo/source-catalog/levels', { url })

// ===== 竞品管理（后端 /geo/competitors/*） =====
export const listCompetitors = () =>
  http.get('/api/geo/competitors')

export const getCompetitor = (id) =>
  http.get(`/api/geo/competitors/${id}`)

export const createCompetitor = (data) =>
  http.post('/api/geo/competitors', data)

export const updateCompetitor = (id, data) =>
  http.put(`/api/geo/competitors/${id}`, data)

export const deleteCompetitor = (id) =>
  http.delete(`/api/geo/competitors/${id}`)
