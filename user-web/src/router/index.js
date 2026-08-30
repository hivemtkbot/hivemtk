import { createRouter, createWebHashHistory } from 'vue-router'
import Layout from '@/layout/Layout.vue'
import { isInitialized } from '@/utils/initHelper'
import { useUserStore } from '@/stores/user'
import { ElMessage } from 'element-plus'

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
  // R48 T1: 公开帮助中心门户（免登录）
  {
    path: '/help-center',
    name: 'HelpCenter',
    component: () => import('@/views/public/HelpCenter.vue'),
    meta: { title: '帮助中心', requiresAuth: false, public: true, hideLayout: true }
  },
  // 公开嵌入聊天窗（被第三方网站 iframe 加载）
  // 私域部署：URL 路径用 channel_ref（兼容 app_key），缺失默认 default
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
  // 兜底路由：访问 /chat/embed 或 /chat/embed/ 时统一重定向到 default
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
  'ragProductConfig', 'scriptTemplate', 'userSegment',
  'community', 'unifiedMessage', 'platformAccount', 'messageHub',
  // 销冠 SOP 智能体相关模块（意图识别 / 对话记忆 / SOP 智能体）
  'intentRecognition', 'dialogueMemory', 'sopAgent',
  // 触达管道 / 统一收件箱 / 企微账号
  // R1-D1 修复: unifiedInbox 模块已随后端 W-3 废弃一并摘除
  'reachPipeline', 'wecomAccount',
  'whatsappCloud',
  'dingtalkApp',
  // 新增模块:LLM 路由 / 标签分层 / 转化漏斗 / AI 产能
  'llmRouting', 'tagSegmentation', 'conversionFunnel',
  'aiProductivity',
  // 知识库管理(导入/统计/OpenAPI)
  'knowledge',
  // 多 AI 智能体架构（智能体管理 / 渠道绑定 / 客服挂载）
  'aiAgent',
  // FAQ 知识库 / SOP 模板管理（知识库管理）
  'faq', 'sopTemplateKb',
  // 知识库统一管理（RAG/FAQ/SOP 树形多选 + 反向追溯）
  'knowledgeBase',
  // 资产市场
  'assetMarket',
  // 资产包（低代码 Playground / 商户编辑器）
  'assetBundle',
  // 客服子功能 (坐席状态 / 快捷回复 / 会话标签 / AI 建议)
  'customerService',
  // 客服 Web Widget 渠道管理
  'chatChannel',
  // G9: 异议处理 / 销冠画像独立 UI
  'objection', 'persona',
  // G10: 客户旅程大屏
  'customerJourney',
  // G13: 备份恢复 / 安全审计
  'backup', 'securityAudit',
  // 工作流编排（可视化工作流编辑器）
  'workflowOrchestrator',
  // 飞书账号管理（配合 reach.feishu.send 工具）
  'feishu',
  // 置信度/拟人度/反馈学习 统一管理面板
  'tuning',
  // 阶段 4：人员管理（v3.1 §3.1）
  'systemUser',
  // 阶段 5：角色管理（v3.1 §3.2）
  'role',
  // 阶段 6：授权管理（v3.1 §3.4）
  'permission',
  // 多语言方案：术语表管理 + 多语言监控看板
  'glossary', 'i18nStats',
  // OPT-UX-03: 跨平台一键发布
  'crossPublish',
  // OPT-UX-01/02: 运维总览 + AI 销冠驾驶舱
  'ops',
  // GEO 智能优化（关键词蒸馏 / 内容创作 / 文章优化 / 多模型验证 / 数据报表 / 配置）
  'geoTools',
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
    // R1-D1 修复: 原默认落地页 /unifiedInbox/list 已废弃,改指统一消息中心
    redirect: '/messageHub/list',
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
  'dingtalk-app': 'dingtalkApp',
  // 多语言：/i18n/* 路径首段与模块名 i18nStats 不一致，需显式映射
  'i18n': 'i18nStats',
  // 知识库管理模块 URL 路径首段与模块名不一致映射
  'sop-template': 'sopTemplateKb',
  // OPT-UX-01/02: 运维总览 / AI 销冠驾驶舱
  'ops-overview': 'ops',
  'sales-cockpit': 'ops',
  // OPT-UX-03: 跨平台一键发布
  'cards': 'crossPublish',
  // 工作流编排：URL 首段 workflow-orchestrator (kebab-case) → 模块名 workflowOrchestrator (camelCase)
  'workflow-orchestrator': 'workflowOrchestrator',
  // GEO 智能优化：URL 首段 geo-tools (kebab-case) → 模块名 geoTools (camelCase)
  'geo-tools': 'geoTools',
  // GEO 决策链报表：路径首段 geo → geoTools 模块（已合并到 geoTools.js）
  'geo': 'geoTools',
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
    const lazyFn = lazyModule(moduleName)
    if (!lazyFn) return false
    const mod = await lazyFn()
    // USR-PF-02: 加 Error Boundary 包装懒加载失败
    if (!mod || !mod.default) {
      console.warn(`[router] 模块 ${moduleName} 加载失败或无 default 导出`)
      return false
    }
    const routes = Array.isArray(mod.default) ? mod.default : [mod.default]
    for (const r of routes) {
      try { router.addRoute('Layout', r) } catch (e) { /* 重复注册容错 */ }
    }
    loadedModules.add(moduleName)
    return true
  } catch (e) {
    console.error(`[router] 模块 ${moduleName} 加载异常`, e)
    return false
  }
}

router.beforeEach(async (to, from, next) => {
  // 懒加载对应模块的路由
  const routeLoaded = await ensureRouteLoaded(to.path)

  // 兼容修复：/system/ 命名空间下的某些路径定义在非 system 模块中
  // 例如 /system/rag-product-config 实际在 ragProductConfig 模块
  // /system/users → systemUser, /system/roles → role, /system/permissions → permission
  // 当首次访问 system 路径且未匹配到路由时，尝试加载所有相关模块
  if (to.path.startsWith('/system/')) {
    const extraModules = ['ragProductConfig', 'systemUser', 'role', 'permission']
    for (const modName of extraModules) {
      if (loadedModules.has(modName)) continue
      try {
        const mod = await lazyModule(modName)()
        const moduleRoutes = mod.default || mod
        if (Array.isArray(moduleRoutes)) {
          moduleRoutes.forEach(route => {
            router.addRoute('Layout', route)
            loadedRoutes.push(route)
          })
        }
        loadedModules.add(modName)
      } catch (err) {
        console.error(`[Lazy Router] Failed to load ${modName}:`, err)
      }
    }
  }

  // 公开页面：setup / login / chat/embed（被第三方网站 iframe 加载）不需要登录态
  const isPublicPath = to.path === '/setup' || to.path === '/login' || to.path.startsWith('/chat/embed')
  const isPublic = to.meta?.public === true || isPublicPath

  if (!isPublic) {
    // 未初始化 → 跳转初始化向导
    if (!isInitialized()) {
      next('/setup')
      return
    }
    const userStore = useUserStore()
    // 默认拒绝：未显式声明 public 的路由均要求登录态
    if (!userStore.isLoggedIn) {
      next('/login')
      return
    }
    // requiresAdmin 守卫：仅 admin 角色可访问
    if (to.meta?.requiresAdmin && userStore.role !== 'admin') {
      ElMessage.error('无权访问（403）：仅管理员可使用该功能')
      next({ name: 'NotFound', query: { status: '403', from: to.fullPath }, replace: true })
      return
    }
  }

  // 路由已按需加载则重放一次以匹配新注册路由，否则直接放行
  if (routeLoaded) {
    next({ path: to.path, replace: true })
  } else {
    // OPT-FE-10：ensureRouteLoaded 失败时跳转 NotFound
    // 旧行为：放行 → 用户看到空白页
    // 新行为：放行但路径不存在则跳 NotFound
    if (to.matched.length === 0) {
      next({ name: 'NotFound', replace: true })
    } else {
      next()
    }
  }
})
export default router
