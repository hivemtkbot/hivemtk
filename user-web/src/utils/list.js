export function toList(v) {
  if (Array.isArray(v)) return v
  if (v && typeof v === 'object') {
    if (Array.isArray(v.list)) return v.list
    if (Array.isArray(v.items)) return v.items
    if (Array.isArray(v.stages)) return v.stages
    if (Array.isArray(v.data)) return v.data
    return [];
  }
  return []
}
