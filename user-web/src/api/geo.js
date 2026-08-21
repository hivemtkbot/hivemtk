import { http } from '@/utils/request'

// GEO 智能优化 API - 匹配后端 /api/geo/* 路径
// 迁移自 AIGEOTOOLS（Streamlit GEO 内容优化工具），保留原始能力划分：
// 关键词蒸馏 / 自动创作 / 文章优化 / 多模型验证 / 数据报表 / 配置优化

export const geoApi = {
  // === 关键词蒸馏 ===
  // 关键词挖掘（AI 生成 / 托词组合 / 混合模式）
  // data: { seed_words, mode, brand_name, advantages }
  mineKeywords(data) {
    return http.post('/api/geo/keywords/mine', data)
  },

  // 语义扩展（基于已有关键词扩展近义/长尾词）
  // data: { keywords, brand_name, expand_mode }
  semanticExpand(data) {
    return http.post('/api/geo/keywords/expand', data)
  },

  // 话题聚类（按主题对关键词聚类，分析覆盖与盲区）
  // data: { keywords, brand_name }
  topicCluster(data) {
    return http.post('/api/geo/keywords/cluster', data)
  },

  // 关键词列表（分页 / 搜索）
  // params: { search?, source?, page, limit }
  getKeywordList(params) {
    return http.get('/api/geo/keywords/list', params)
  },

  // 删除关键词
  deleteKeyword(id) {
    return http.delete(`/api/geo/keywords/${id}`)
  },

  // === 内容创作 ===
  // 生成内容（按关键词/品牌/优势/平台/字数/风格生成文章）
  // data: { keyword, brand_name, advantages, model, word_count, style }
  generateContent(data) {
    return http.post('/api/geo/content/generate', data)
  },

  // 内容打分（对生成内容进行 GEO 质量评分）
  // data: { content, brand_name, keyword }
  scoreContent(data) {
    return http.post('/api/geo/content/score', data)
  },

  // === 文章优化 ===
  // 优化内容（结构化 / 可引用 / 自然植入品牌）
  // data: { article_id, content, brand_name, advantages, model }
  optimizeContent(data) {
    return http.post('/api/geo/content/optimize', data)
  },

  // E-E-A-T 强化（专业性 / 经验性 / 权威性 / 可信度）
  // data: { content, brand_name, advantages }
  enhanceEEAT(data) {
    return http.post('/api/geo/content/eeat', data)
  },

  // 结构化 Schema 生成（JSON-LD）
  generateSchema(data) {
    return http.post('/api/geo/content/schema', data)
  },

  // 查重 / 唯一性检测
  checkUniqueness(data) {
    return http.post('/api/geo/content/uniqueness', data)
  },

  // 文章列表（分页）
  // params: { page, limit }
  getArticleList(params) {
    return http.get('/api/geo/content/list', params)
  },

  // 获取文章详情
  getArticleByID(id) {
    return http.get(`/api/geo/content/${id}`)
  },

  // === 多模型验证 ===
  // 多模型验证（跨 LLM 验证品牌提及率 / 竞品对比）
  // data: { article_id, query, brand_name, models }
  verifyArticle(data) {
    return http.post('/api/geo/verification/verify', data)
  },

  // 负面监控（生成负面查询并分析品牌负面提及）
  // data: { brand_name }
  monitorNegative(data) {
    return http.post('/api/geo/verification/negative', data)
  },

  // 获取验证结果
  getVerifyResults(articleId) {
    return http.get(`/api/geo/verification/results/${articleId}`)
  },

  // === 数据报表 ===
  // 报表汇总（总文章 / 关键词 / 优化 / 验证 / 成本）
  // params: { start_date?, end_date? }
  getReport(params) {
    return http.get('/api/geo/reports/summary', params)
  },

  // ROI 分析
  getROI(params) {
    return http.get('/api/geo/reports/roi', params)
  },

  // API 成本明细（按 provider / model 拆分）
  getAPICosts(params) {
    return http.get('/api/geo/reports/api-costs', params)
  },

  // === 配置优化 ===
  // 获取 GEO 配置（品牌 / 优势 / 竞品 / 默认模型 / 验证模型）
  getConfig() {
    return http.get('/api/geo/config')
  },

  // 更新配置
  updateConfig(data) {
    return http.put('/api/geo/config', data)
  },

  // 优化配置（LLM 给出配置优化建议）
  optimizeConfig(data) {
    return http.post('/api/geo/config/optimize', data)
  }
}

export default geoApi
