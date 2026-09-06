import { ref, computed, watch, onMounted } from 'vue';
import { evaluateFlag } from '@/api/featureFlag'

const _cache = new Map();
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
      value.value = defaultValue;
    } finally {
      loading.value = false
    }
  }

  function refresh() {
    _cache.delete(key)
    return fetch()
  }

  onMounted(fetch)
  watch(attributes, fetch, { deep: true });

  return { isOn, value, loading, error, refresh }
}

export function clearFeatureFlagCache() {
  _cache.clear()
}

export default { useFeatureFlag, clearFeatureFlagCache }
