<template>
  <el-config-provider :locale="elLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import en from 'element-plus/es/locale/lang/en'
import ja from 'element-plus/es/locale/lang/ja'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import i18n from '@/i18n'
import { elLocaleCode, applyDirection } from '@/i18n/locale'
import { http } from '@/utils/request'

const elMap = { 'zh-cn': zhCn, en, ja }
const elLocale = computed(() => elMap[elLocaleCode(i18n.global.locale.value)] || zhCn)

const router = useRouter()
const t = i18n.global.t

// 应用启动时检查后端初始化状态
// 对应 MERCHANT_INITIALIZATION_FLOW.md §4.1
onMounted(async () => {
  applyDirection(i18n.global.locale.value)
  try {
    // 查询后端初始化状态
    const resp = await http.get('/api/system/init-status')
    const status = resp?.data || resp
    if (!status) return

    // 关键修复：以后端真实状态同步前端初始化标志
    // 避免后端已初始化但 localStorage 未设置导致路由守卫反复跳 /setup
    if (status.initialized) {
      localStorage.setItem('system_initialized', 'true')
    } else {
      localStorage.removeItem('system_initialized')
    }

    const currentPath = router.currentRoute.value.path

    // 未初始化 + 不在 setup 页 → 引导到 /setup
    if (!status.initialized) {
      if (currentPath !== '/setup' && currentPath !== '/login') {
        ElMessage.info(t('system.completeInitFirst'))
        router.replace('/setup')
      }
    }
  } catch (e) {
    // 静默失败 - 可能是未运行后端或权限问题
    console.warn('[App] init-status check failed:', e)
  }
})
</script>

<style>
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: #2c3e50;
  height: 100vh;
  margin: 0;
  padding: 0;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body {
  height: 100%;
  margin: 0;
  padding: 0;
}
</style>
