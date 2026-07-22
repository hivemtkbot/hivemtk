// API 调用冒烟测试辅助工具（非测试文件，不被 vitest include 匹配）
// 用于在单测中验证每个 API 导出函数可被调用并正确触发 HTTP 请求。

// 展开模块的导出：顶层函数直接收集；导出对象（如 platformAccountApi）递归收集其方法。
export function flattenApiExports(mod) {
  const out = []
  for (const [key, val] of Object.entries(mod || {})) {
    if (typeof val === 'function') {
      out.push([key, val])
    } else if (val && typeof val === 'object' && !Array.isArray(val)) {
      for (const [k2, v2] of Object.entries(val)) {
        if (typeof v2 === 'function') out.push([`${key}.${k2}`, v2])
      }
    }
  }
  return out
}

// 清空所有 mock 的调用记录
export function clearMocks(m) {
  const fns = [
    m.request, m.request.get, m.request.post, m.request.put, m.request.delete, m.request.upload,
    m.http.get, m.http.post, m.http.put, m.http.delete, m.http.upload,
    m.axios.get, m.axios.post, m.axios.put, m.axios.delete
  ]
  fns.forEach((fn) => fn && fn.mockClear())
}

// 收集本轮所有 HTTP 调用的首个参数（url 或 { url }）
export function getCalls(m) {
  return [
    ...m.request.mock.calls.map((c) => c[0]),
    ...m.request.get.mock.calls.map((c) => c[0]),
    ...m.request.post.mock.calls.map((c) => c[0]),
    ...m.request.put.mock.calls.map((c) => c[0]),
    ...m.request.delete.mock.calls.map((c) => c[0]),
    ...m.request.upload.mock.calls.map((c) => c[0]),
    ...m.http.get.mock.calls.map((c) => c[0]),
    ...m.http.post.mock.calls.map((c) => c[0]),
    ...m.http.put.mock.calls.map((c) => c[0]),
    ...m.http.delete.mock.calls.map((c) => c[0]),
    ...m.http.upload.mock.calls.map((c) => c[0]),
    ...m.axios.get.mock.calls.map((c) => c[0]),
    ...m.axios.post.mock.calls.map((c) => c[0]),
    ...m.axios.put.mock.calls.map((c) => c[0]),
    ...m.axios.delete.mock.calls.map((c) => c[0])
  ]
}

// 校验 HTTP 调用的 url 是否有效：
//  - 返回 null 表示本轮没有发起任何 HTTP 调用（可能是合法 stub）
//  - 返回 true  表示所有调用都携带了非空 url
//  - 返回 false 表示存在空的 url（真实 bug，如漏写 url）
export function hasValidUrl(calls) {
  if (calls.length === 0) return null
  return calls.every((a) => {
    if (typeof a === 'string') return a.trim().length > 0
    if (a && typeof a.url === 'string') return a.url.trim().length > 0
    return false
  })
}

// 多种参数组合，兼容不同签名（无参 / 单参 id / 对象 / 多参等）
export const ARG_SETS = [
  [],
  [1],
  ['x'],
  [{}],
  [{ id: 1 }],
  [{ data: {} }],
  [1, {}],
  [1, 'x'],
  [{}, {}],
  ['x', {}],
  [1, 1],
  [null],
  [undefined]
]
