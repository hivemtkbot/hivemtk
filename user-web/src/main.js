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
//   - 详见 audit/USER_PROJECT_INSPECTION_BRAINSTORM.md P1-1。

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

app.mount('#app')
