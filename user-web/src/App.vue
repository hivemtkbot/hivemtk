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

    // 未初始化 + 不在 setup / login / chat-embed 页 → 引导到 /setup
    // 公开的 chat embed 页面（被第三方网站 iframe 加载）即使后端未初始化也应继续展示，
    // 否则会重定向到 /setup，导致 iframe 内出现完整 Layout（顶部 LanguageSwitcher + 侧边栏 + 菜单）。
    // 聊天本身依赖的后端 API（/api/chat/public/*、/api/ws/visitor）都是公开的，不依赖初始化。
    const isPublicEmbedPath =
      currentPath.startsWith('/chat/embed') ||
      currentPath === '/setup' ||
      currentPath === '/login'
    if (!status.initialized) {
      if (!isPublicEmbedPath) {
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
