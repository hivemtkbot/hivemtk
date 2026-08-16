/**
 * 知识库多租户隔离强化（USR-KB-02）
 * 借鉴：PostgreSQL Row-Level Security
 *
 * 三道防线：
 * 1. HTTP 层：所有 API 强制带 merchant_id（来自 token）
 * 2. Repository 层：每个查询自动 WHERE merchant_id = ?
 * 3. 数据库层：PostgreSQL RLS Policy
 */

// 前端防御：所有 API 调用自动带 X-Merchant-Id 头
// 实际由 request.js 拦截器自动注入

// 知识库安全策略（前端展示给用户）
export const KB_SECURITY_POLICY = {
  // 跨租户访问：禁止
  crossTenantAccess: 'forbidden',
  // 公开 KB：允许（owner_id = 0 表示系统公开）
  publicAccess: 'allowed_with_warning',
  // Playground：自动加租户过滤
  playgroundAutoFilter: true,
  // OpenAPI Token：必须绑定租户
  openApiTokenBinding: 'required',
  // 引用溯源：必须携带 source
  requireSource: true
}

// 检查知识库访问权限
export const checkKbAccess = (kb, currentMerchantId) => {
  if (!kb) return { allowed: false, reason: 'not_found' }
  // 公开知识库
  if (kb.is_public || kb.owner_id === 0) {
    return { allowed: true, scope: 'public' }
  }
  // 同租户
  if (kb.merchant_id === currentMerchantId || kb.owner_id === currentMerchantId) {
    return { allowed: true, scope: 'tenant' }
  }
  return { allowed: false, reason: 'cross_tenant' }
}

// Playground 评估时自动注入租户过滤
export const buildPlaygroundFilter = (currentMerchantId) => ({
  merchant_id: currentMerchantId,
  include_public: true
})
