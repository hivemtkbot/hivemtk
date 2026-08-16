/**
 * Feature Flag Composable（USR-AI-05）
 * 借鉴：https://docs.growthbook.io/lib/vue
 *
 * 用法：
 *   const { isOn, value, loading, error } = useFeatureFlag('new_dashboard')
 *   if (isOn.value) { ... }
 *
 *   <script setup>
 *   const { isOn: showBanner } = useFeatureFlag('show-banner')
 *   </script>
 */
import { ref, computed, watch, onMounted } from 'vue'
import { evaluateFlag } from '@/api/featureFlag'

const _cache = new Map() // key -> { value, attributesHash, expiresAt }
const CACHE_TTL_MS = 30 * 1000

function hashAttributes(attrs) {
  return JSON.stringify(attrs || {})
}

export function useFeatureFlag(key, options = {}) {
  const {
    attributes = () => ({}),
    defaultValue = false,
    skip = false
  } = options

  const value = ref(defaultValue)
  const loading = ref(false)
  const error = ref(null)
  const isOn = computed(() => Boolean(value.value))

  async function fetch() {
    if (skip || !key) return
    const attrs = attributes()
    const attrsHash = hashAttributes(attrs)
    const cached = _cache.get(key)
    if (cached && cached.attributesHash === attrsHash && cached.expiresAt > Date.now()) {
      value.value = cached.value
      return
    }
    loading.value = true
    error.value = null
    try {
      const res = await evaluateFlag(key, attrs)
      const v = res?.value ?? defaultValue
      value.value = typeof v === 'object' ? JSON.parse(JSON.stringify(v)) : v
      _cache.set(key, { value: value.value, attributesHash: attrsHash, expiresAt: Date.now() + CACHE_TTL_MS })
    } catch (e) {
      error.value = e
      value.value = defaultValue // 降级到默认值
    } finally {
      loading.value = false
    }
  }

  function refresh() {
    _cache.delete(key)
    return fetch()
  }

  onMounted(fetch)
  // 监听 attributes 变化重新拉取
  watch(attributes, fetch, { deep: true })

  return { isOn, value, loading, error, refresh }
}

// 全局清理（用于登出 / 测试）
export function clearFeatureFlagCache() {
  _cache.clear()
}

export default { useFeatureFlag, clearFeatureFlagCache }
