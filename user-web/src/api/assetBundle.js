import request from '@/utils/request'

// ============================================================================
// 资产包（AssetBundle）API 客户端
//
// 文档依据: docs/企业级架构优化/资产包模式.md
// 后端路由: /api/asset-bundle/* (controller/asset_bundle.go)
//
// 设计:
//   - 开发者模式 (Playground): 直接绑定 messages 数组
//   - 商户模式 (低代码): 表单 → 后端翻译成 messages
//   - 热插拔 (D1): 立即生效的启用/禁用
// ============================================================================

// ==================== 基础 CRUD ====================

// 创建资产包
export const createBundle = (data) =>
  request({ url: '/api/asset-bundle', method: 'post', data })

// 更新资产包
export const updateBundle = (id, data) =>
  request({ url: `/api/asset-bundle/${id}`, method: 'put', data })

// 按 ID 查询
export const getBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}`, method: 'get' })

// 按 AssetID（业务键）查询
export const getBundleByAssetID = (aid) =>
  request({ url: `/api/asset-bundle/by-aid/${aid}`, method: 'get' })

// 分页查询
export const listBundles = (data) =>
  request({ url: '/api/asset-bundle/list', method: 'post', data })

// 启用资产包（draft → active）
export const publishBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}/publish`, method: 'post' })

// 提交平台审核上架（开发者上架链路）
export const submitToPlatform = (id) =>
  request({ url: `/api/asset-bundle/${id}/submit-platform`, method: 'post' })

// 归档资产包
export const archiveBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}/archive`, method: 'post' })

// 软删除
export const deleteBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}`, method: 'delete' })

// ==================== Weave 织布算法 ====================

// 织布：资产包 + RAG + 历史 + 当前提问 → 最终 messages 数组
export const weaveBundle = (data) =>
  request({ url: '/api/asset-bundle/weave', method: 'post', data })

// ==================== 商户低代码模式 ====================

// 商户表单保存（前端表单 → 后端翻译成 messages 数组）
export const merchantSave = (data) =>
  request({ url: '/api/asset-bundle/merchant-save', method: 'post', data })

// 商户表单解析（messages 数组 → 前端表单回显）
export const merchantParse = (aid) =>
  request({ url: `/api/asset-bundle/merchant-parse/${aid}`, method: 'post' })

// ==================== 热插拔（方向 D1，立即生效） ====================

// 热启用资产包
export const enableBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}/enable`, method: 'post' })

// 热禁用资产包
export const disableBundle = (id) =>
  request({ url: `/api/asset-bundle/${id}/disable`, method: 'post' })

// 查询已热启用的资产包列表
export const listEnabledBundles = () =>
  request({ url: '/api/asset-bundle/enabled/list', method: 'post' })
