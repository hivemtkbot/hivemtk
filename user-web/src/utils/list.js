// 统一的列表数据归一化：保证 el-table 的 :data 永远是数组，
// 避免后端返回对象 / {"error":"not found"} / 分页包裹 {list,...} 时
// 触发 Element Plus 内部 data.includes is not a function 崩溃。
export function toList(v) {
  if (Array.isArray(v)) return v
  if (v && typeof v === 'object') {
    if (Array.isArray(v.list)) return v.list
    if (Array.isArray(v.items)) return v.items
    if (Array.isArray(v.stages)) return v.stages
    if (Array.isArray(v.data)) return v.data
    // 单个对象（如流失分析单条记录）不强行包成数组，返回空以避免表格崩溃
    return []
  }
  return []
}
