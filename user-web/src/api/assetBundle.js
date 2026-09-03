import { http } from '@/utils/request';

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
  http.post('/api/asset-bundle', data)

// 更新资产包
export const updateBundle = (id, data) =>
  http.put(`/api/asset-bundle/${id}`, data)

// 按 ID 查询
export const getBundle = (id) =>
  http.get(`/api/asset-bundle/${id}`)

// 按 AssetID（业务键）查询
export const getBundleByAssetID = (aid) =>
  http.get(`/api/asset-bundle/by-aid/${aid}`)

// 分页查询
export const listBundles = (data) =>
  http.post('/api/asset-bundle/list', data)

// 启用资产包（draft → active）
export const publishBundle = (id) =>
  http.post(`/api/asset-bundle/${id}/publish`)

// 提交平台审核上架（开发者上架链路）
export const submitToPlatform = (id) =>
  http.post(`/api/asset-bundle/${id}/submit-platform`)

// 归档资产包
export const archiveBundle = (id) =>
  http.post(`/api/asset-bundle/${id}/archive`)

// 软删除
export const deleteBundle = (id) =>
  http.delete(`/api/asset-bundle/${id}`)

// ==================== Weave 织布算法 ====================

// 织布：资产包 + RAG + 历史 + 当前提问 → 最终 messages 数组
export const weaveBundle = (data) =>
  http.post('/api/asset-bundle/weave', data)

// ==================== 商户低代码模式 ====================

// 商户表单保存（前端表单 → 后端翻译成 messages 数组）
export const merchantSave = (data) =>
  http.post('/api/asset-bundle/merchant-save', data)

// 商户表单解析（messages 数组 → 前端表单回显）
export const merchantParse = (aid) =>
  http.post(`/api/asset-bundle/merchant-parse/${aid}`)

// ==================== 热插拔（方向 D1，立即生效） ====================

// 热启用资产包
export const enableBundle = (id) =>
  http.post(`/api/asset-bundle/${id}/enable`)

// 热禁用资产包
export const disableBundle = (id) =>
  http.post(`/api/asset-bundle/${id}/disable`)

// 查询已热启用的资产包列表
export const listEnabledBundles = () =>
  http.post('/api/asset-bundle/enabled/list')

// ==================== 封面图上传 ====================

// 封面图上传：multipart form-data → POST /api/upload
// 后端返回 { url: "/files/attachments/YYYY/MM/uuid.jpg", filename, size }
export const uploadCover = (file) => {
  const fd = new FormData()
  fd.append('file', file)
  return http.post('/api/upload', fd, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
