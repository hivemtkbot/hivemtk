import { createRouter, createWebHashHistory } from 'vue-router'
import Layout from '@/layout/Layout.vue'
import { isInitialized } from '@/utils/initHelper'
import { useUserStore } from '@/stores/user'

// 路由模块 - 使用 import.meta.glob 实现懒加载（Vite 兼容）
// Vite 在构建时会静态分析 glob 模式，将所有匹配的模块打包进产物
const routeModules = import.meta.glob('./modules/*.js')

const lazyModule = (name) => {
  const key = `./modules/${name}.js`
  return routeModules[key] || null
}

// 初始化路由
const initRoutes = [
  {
    path: '/setup',
    name: 'InitSetup',
    component: () => import('@/views/setup/InitSetup.vue'),
    meta: { title: '系统初始化' }
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录' }
  },
  // P0-10 ADR-010: 公开嵌入聊天窗（被第三方网站 iframe 加载）
  // 私域部署（2026-07-17 优化）：URL 路径用 channel_ref（兼容 app_key），缺失默认 default
  {
    path: '/chat/embed/default',
    name: 'ChatEmbed',
    component: () => import('@/views/chat/embed/Index.vue'),
    meta: { title: '在线客服', requiresAuth: false, public: true, hideLayout: true }
  },
  {
    path: '/chat/embed/:channel_ref',
    name: 'ChatEmbedChannel',
    component: () => import('@/views/chat/embed/Index.vue'),
    meta: { title: '在线客服', requiresAuth: false, public: true, hideLayout: true }
  },
  // 2026-07-17: 兜底路由，修复 /chat/embed 或 /chat/embed/ 直接访问 404 / 跳登录的问题
  //   之前只有 /chat/embed/default 和 /chat/embed/:channel_ref，
  //   访问 /chat/embed（无 default）会被 NotFound 拦截，
  //   访问 /chat/embed/ 会被 catch-all 路由拦截到登录页。
  //   统一重定向到 default 即可保证任意形式的 URL 都能进入聊天窗。
  {
    path: '/chat/embed',
    redirect: '/chat/embed/default'
  },
  {
    path: '/chat/embed/',
    redirect: '/chat/embed/default'
  }
]

// 路由模块 - 懒加载 (按访问时下载)
const moduleNames = [
  'email', 'telegram', 'whatsapp', 'clue', 'system',
  'domainPool', 'shortLink', 'douyinCard', 'xiaohongshuCard', 'kuaishouCard',
  'xianyuCard', 'sms', 'livecode', 'tiktok',
  'abExperiment', 'batchOperation', 'churnPrediction',
  'customReport', 'customer360', 'customerEvent', 'customerSession',
  // OneID 客户身份统一 (身份归一化 / 冲突解决)
  'oneid',
  'dashboardScreen', 'integration', 'marketingFlow', 'operationLog',
  'ragProductConfig', 'scriptTemplate', 'teamUser', 'userSegment',
  'community', 'unifiedMessage', 'platformAccount', 'messageHub',
  // 销冠 SOP 智能体相关模块（意图识别 / 对话记忆 / SOP 智能体）
  'intentRecognition', 'dialogueMemory', 'sopAgent',
  // 触达管道 / 统一收件箱 / 企微账号
  'reachPipeline', 'unifiedInbox', 'wecomAccount',
  'whatsappCloud',
  'dingtalkApp',
  // P1/P2 新增模块:LLM 路由 / 标签分层 / 转化漏斗 / AI 产能
  'llmRouting', 'tagSegmentation', 'conversionFunnel',
  'aiProductivity',
  // 知识库管理(导入/统计/OpenAPI)
  'knowledge',
  // 多 AI 智能体架构（智能体管理 / 渠道绑定 / 客服挂载）
  'aiAgent',
  // 资产市场
  'assetMarket',
  // 资产包（低代码 Playground / 商户编辑器）
  'assetBundle',
  // 客服子功能 (坐席状态 / 快捷回复 / 会话标签 / AI 建议)
  'customerService',
  // P0-10 ADR-010: 客服 Web Widget 渠道管理
  'chatChannel',
  // P2-1 G9: 异议处理 / 销冠画像独立 UI
  'objection', 'persona',
  // P2-2 G10: 客户旅程大屏
  'customerJourney',
  // P2-5 G13: 备份恢复 / 安全审计
  'backup', 'securityAudit',
  // 飞书账号管理（配合 reach.feishu.send 工具）
  'feishu',
  // 置信度/拟人度/反馈学习 统一管理面板
  'tuning',
  // 阶段 5：角色管理（v3.1 §3.2）
  'role',
  // 阶段 6：授权管理（v3.1 §3.4）
  'permission'
]

// 同步注册的路由 (始终加载 - 用于 SSR / 初始 SEO)
const eagerLoadedRoutes = []
// 懒加载的路由 (按需下载)
const lazyRoutes = []

const routes = [
  {
    path: '/',
    name: 'Layout',
    component: Layout,
    redirect: '/unifiedInbox/list',
    children: [
      // 路由将在 router.beforeEach 中按需注册 (懒加载)
      ...eagerLoadedRoutes,
      // 个人资料 & 通知中心（常驻路由，避免被懒加载机制漏掉）
      {
        path: 'profile',
        name: 'Profile',
        component: () => import('@/views/Profile.vue'),
        meta: { title: '个人资料', requiresAuth: true }
      },
      {
        path: 'notifications',
        name: 'Notifications',
        component: () => import('@/views/Notifications.vue'),
        meta: { title: '通知中心', requiresAuth: true }
      }
    ],
  },
  ...initRoutes,
  {
    path: '/telegram/group',
    redirect: '/telegram/account'
  },
  // 兜底重定向：兼容 /oneid 直达（实际路由挂在 Layout 下为 /oneid/list）
  {
    path: '/oneid',
    redirect: '/oneid/list'
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('@/views/NotFound.vue'),
    meta: { title: '页面不存在' }
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// 已加载的路由模块缓存
const loadedModules = new Set()
const loadedRoutes = []

/**
 * 懒加载路由模块
 * 根据当前访问路径，动态加载对应的路由模块文件
 * @param {string} path - 当前路由路径
 * @returns {boolean} 是否新加载了路由模块
 */
const pathToModule = {
  'douyin': 'douyinCard',
  'xiaohongshu': 'xiaohongshuCard',
  'kuaishou': 'kuaishouCard',
  'xianyu': 'xianyuCard',
  // 卡片统计为各卡片模块内的顶层路由（/xxx-card-stats/:id），首段与模块名不同，
  // 直接深链访问需显式映射，避免懒加载不到对应模块导致 404
  'douyin-card-stats': 'douyinCard',
  'xiaohongshu-card-stats': 'xiaohongshuCard',
  'kuaishou-card-stats': 'kuaishouCard',
  'xianyu-card-stats': 'xianyuCard',
  // 资产市场 / 资产包：kebab-case 首段与 camelCase 模块名不匹配
  'asset-market': 'assetMarket',
  'asset-bundle': 'assetBundle',
  // 置信度 / 拟人度 / 反馈学习 面板统一定义在 tuning 模块
  'confidence': 'tuning',
  'humanize': 'tuning',
  'feedbackLoop': 'tuning',
  // WhatsApp Cloud（Meta 商业 API）路由首段 kebab-case → camelCase 模块名
  'whatsapp-cloud': 'whatsappCloud',
  // 钉钉应用（企业内部应用，支持回调收消息）
  'dingtalk-app': 'dingtalkApp'
}

async function ensureRouteLoaded(path) {
  // 提取第一段路径作为模块名
  const pathSegments = path.split('/').filter(Boolean)
  if (pathSegments.length === 0) return false

  const moduleName = pathToModule[pathSegments[0]] || pathSegments[0]
  if (!moduleNames.includes(moduleName)) return false
  if (loadedModules.has(moduleName)) return false

  try {
    // 动态加载对应模块的路由配置
    const mod = await lazyModule(moduleName)()
    const moduleRoutes = mod.default || mod

    if (Array.isArray(moduleRoutes)) {
      // 使用 router.addRoute 动态添加路由（Vue Router 4 正确方式）
      moduleRoutes.forEach(route => {
        router.addRoute('Layout', route)
        loadedRoutes.push(route)
      })
    }
    loadedModules.add(moduleName)
    return true
  } catch (err) {
    console.error(`[Lazy Router] Failed to load module ${moduleName}:`, err)
    return false
  }
}

router.beforeEach(async (to, from, next) => {
  // 懒加载对应模块的路由
  const routeLoaded = await ensureRouteLoaded(to.path)

  // 兼容修复：/system/ 命名空间下的某些路径定义在非 system 模块中
  // 例如 /system/rag-product-config 实际在 ragProductConfig 模块
  // 当首次访问 system 路径且未匹配到路由时，尝试加载所有相关模块
  if (to.path.startsWith('/system/') && !loadedModules.has('ragProductConfig')) {
    try {
      const mod = await lazyModule('ragProductConfig')()
      const moduleRoutes = mod.default || mod
      if (Array.isArray(moduleRoutes)) {
        moduleRoutes.forEach(route => {
          router.addRoute('Layout', route)
          loadedRoutes.push(route)
        })
      }
      loadedModules.add('ragProductConfig')
    } catch (err) {
      console.error('[Lazy Router] Failed to load ragProductConfig:', err)
    }
  }

  // 公开页面：setup / login / chat/embed 不需要登录态也不需要初始化检查
  const isPublicPath = to.path === '/setup' || to.path === '/login' || to.path.startsWith('/chat/embed')
  if (isPublicPath) {
    if (routeLoaded) {
      next({ path: to.path, replace: true })
    } else {
      next()
    }
    return
  }

  // 其他路径需要检查初始化 + 登录态
  if (!isInitialized()) {
    next('/setup')
  } else if (to.meta.requiresAuth) {
    const userStore = useUserStore()
    if (userStore.isLoggedIn) {
      next()
    } else {
      next('/login')
    }
  } else {
    if (routeLoaded) {
      next({ path: to.path, replace: true })
    } else {
      next()
    }
  }
})
export default router
