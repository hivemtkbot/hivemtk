import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import pinia from './stores'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import './styles/index.scss'
import { updateRequestConfig, http } from './utils/request'
import i18n from './i18n'
import { applyDirection } from './i18n/locale'

// Element Plus 图标改为按需自动导入：
//   - 由 vite.config.js 中的 unplugin-vue-components + ElementPlusResolver 在编译期
//     扫描模板里使用的 <Edit />、<Search /> 等图标组件,自动注入 import 语句。
//   - 不再在运行期循环注册全量图标,减少首屏 bundle 体积。

applyDirection(i18n.global.locale.value)

const app = createApp(App)

app.use(router)
app.use(pinia)
app.use(ElementPlus)
app.use(i18n)

// 初始化API配置
updateRequestConfig().catch(error => {
  console.error('初始化API配置失败:', error)
})

// OPT-SEC-07：根据当前路由动态调整 CSP frame-ancestors
// /chat/embed 路由需要被第三方网站嵌入（embed-sdk 场景）
// 其他路由必须 frame-ancestors 'none' 防止点击劫持
router.afterEach((to) => {
  const cspMeta = document.querySelector('meta[http-equiv="Content-Security-Policy"]')
  if (!cspMeta) return

  if (to.path.startsWith('/chat/embed')) {
    // 允许被任意 origin 嵌入（embed-sdk 业务场景）
    // 限制:仅 /chat/embed 路由开放,其他路由保持 'none'
    const csp = cspMeta.getAttribute('content')
    const newCsp = csp.replace(/frame-ancestors\s+[^;]+/, "frame-ancestors *")
    cspMeta.setAttribute('content', newCsp)
  } else {
    // 默认严格模式:禁止嵌入
    const csp = cspMeta.getAttribute('content')
    if (csp && !csp.includes("frame-ancestors 'none'")) {
      const newCsp = csp.replace(/frame-ancestors\s+[^;]+/, "frame-ancestors 'none'")
      cspMeta.setAttribute('content', newCsp)
    }
  }
})

app.mount('#app')
