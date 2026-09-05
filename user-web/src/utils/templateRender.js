const VARIABLE_RE = /\{\{\s*([^{}]+?)\s*\}\}/g;
const IF_RE = /\{\{#if\s+([^{}]+?)\}\}([\s\S]*?)\{\{\/if\}\}/g

function lookup(path, ctx) {
  if (!path || !ctx) return undefined
  const parts = path.trim().split('.')
  let v = ctx
  for (const p of parts) {
    if (v == null) return undefined
    v = v[p]
  }
  return v
}

function isTruthy(v) {
  if (v === undefined || v === null) return false
  if (typeof v === 'string') return v.trim().length > 0
  if (typeof v === 'number') return v !== 0
  if (typeof v === 'boolean') return v
  if (Array.isArray(v)) return v.length > 0
  return true
}

export function extractVariables(template) {
  if (!template) return []
  const vars = new Set()
  let m
  const cleaned = template.replace(IF_RE, '');
  const re = new RegExp(VARIABLE_RE.source, 'g')
  while ((m = re.exec(cleaned))) {
    const path = m[1].trim()
    if (!path.startsWith('#') && !path.startsWith('/')) {
      vars.add(path)
    }
  }
  return Array.from(vars)
}

export function render(template, context = {}, options = {}) {
  if (!template) return ''
  const { strict = false, missing = '[?]' } = options

  let out = template.replace(IF_RE, (_, cond, inner) => {
    return isTruthy(lookup(cond, context)) ? inner : ''
  });

  out = out.replace(VARIABLE_RE, (_, path) => {
    const v = lookup(path, context)
    if (v === undefined || v === null) {
      if (strict) throw new Error(`模板变量缺失: ${path}`)
      return missing
    }
    if (typeof v === 'object') return JSON.stringify(v)
    return String(v)
  });

  return out
}

export function validateTemplate(template) {
  const errors = []
  if (!template) return { valid: true, errors }

  const ifOpen = (template.match(/\{\{#if\s+/g) || []).length;
  const ifClose = (template.match(/\{\{\/if\}\}/g) || []).length
  if (ifOpen !== ifClose) {
    errors.push(`{{#if}} 块未闭合: 开启 ${ifOpen} 个，关闭 ${ifClose} 个`)
  }

  const openCount = (template.match(/\{\{/g) || []).length;
  const closeCount = (template.match(/\}\}/g) || []).length
  if (openCount !== closeCount) {
    errors.push(`{{}} 配对不平衡: {{ 共 ${openCount} 个，}} 共 ${closeCount} 个`)
  }

  return { valid: errors.length === 0, errors }
}

export const BUILTIN_VARIABLES = [
  { key: 'customer.name', desc: '客户名称' },
  { key: 'customer.phone', desc: '客户手机号' },
  { key: 'customer.email', desc: '客户邮箱' },
  { key: 'customer.profile.tier', desc: '客户等级（VIP/普通）' },
  { key: 'order.id', desc: '订单 ID' },
  { key: 'order.amount', desc: '订单金额' },
  { key: 'agent.name', desc: '坐席姓名' },
  { key: 'product.title', desc: '商品名称' },
  { key: 'product.price', desc: '商品价格' }
];

export default { render, extractVariables, validateTemplate, BUILTIN_VARIABLES }
