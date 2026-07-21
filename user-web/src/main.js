import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import pinia from './stores'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import './styles/index.scss'
import { updateRequestConfig, http } from './utils/request'
import i18n from './i18n'
import { applyDirection } from './i18n/locale'

applyDirection(i18n.global.locale.value)

const app = createApp(App)

// 注册 Element Plus 图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(router)
app.use(pinia)
app.use(ElementPlus)
app.use(i18n)

// 初始化API配置
updateRequestConfig().catch(error => {
  console.error('初始化API配置失败:', error)
})

app.mount('#app')
