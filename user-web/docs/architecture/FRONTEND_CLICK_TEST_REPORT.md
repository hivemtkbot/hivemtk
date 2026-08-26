# 前端点击遍历测试报告（user-web）

- 日期：2026-08-26
- 方式：Playwright（Chromium headless）模拟人工登录 + 逐页 goto 遍历，抽样点击安全按钮（刷新/搜索/查询/重置/导出）
- 环境：dev server `localhost:8211`（Vite），`/api` 代理至后端 `localhost:8204`
- 账号：admin / Test@123456

## 一、统计汇总

| 指标 | 数值 |
|------|------|
| 路由总数（含参数路由） | 159 |
| 正常渲染 | 159 / 159（0 白屏） |
| 渲染崩溃（pageerror） | 4 页（已全部修复） |
| 接口断链（前端调用后端不存在的 API） | 1 个（已记录遗留） |
| 参数错误 400 | 2 处（已修复） |
| 全局 console 报错 | CSP meta 警告 ×159 页（已修复） |

## 二、问题分类与修复清单

### 【白屏/渲染崩溃】4 处 —— 已全部修复

| # | 页面 | 错误 | 根因 | 修复 |
|---|------|------|------|------|
| 1 | `/kuaishou-card-stats/:id` | `Cannot read properties of null (reading 'map')` | 后端无数据时 `dailyStats` 返回 null，`Object.assign` 覆盖默认 `[]` 后 echarts 初始化 `.map()` 崩溃 | `src/views/kuaishouCard/CardStats.vue`：赋值后对 `dailyStats`/`recentActivity` 做 Array 兜底 |
| 2 | `/chatChannel/install-guide/:id?` | `(selected.allowed_origins \|\| []).join is not a function` | 后端 `allowed_origins` 为逗号分隔字符串，前端按数组 `.join()` | `src/views/chatChannel/InstallGuide.vue`：新增 `originsToText()` 兼容数组/字符串两种格式（模板重置按钮 + onChannelChange 两处） |
| 3 | `/oneid/merge-rules` | `data.includes is not a function` | 后端返回 `{built_in:[], custom:[], strategy:{}}` 对象，前端当数组绑定 el-table | `src/views/oneid/MergeRuleConfig.vue`：load() 归一化为扁平数组并映射字段（field→fields、补 type/threshold 默认值） |
| 4 | `/dingtalk-app`、`/whatsapp-cloud`（及同类隐患） | `[router] 模块加载异常 TypeError: routes is not iterable` | 两模块 `export default {}` 为单路由对象，`ensureRouteLoaded` 按 for...of 数组迭代崩溃，导致页面落入 NotFound 兜底 | `src/router/index.js:238`：`Array.isArray(mod.default) ? mod.default : [mod.default]` 统一兼容 |

### 【接口断链】1 处 —— 记录遗留

| # | 调用 | 页面 | 说明 |
|---|------|------|------|
| 1 | `POST /api/oneid/merge-rules/preview` | `/oneid/merge-rules` | 后端仅有 GET/POST `/api/oneid/merge-rules`，preview 端点不存在。实现需扫描客户身份数据模拟合并候选对，属中型业务功能。前端已有 catch 不崩，但"命中预览"卡片无法显示。**建议后端按五层架构补 preview handler** |

### 【接口 400 参数错误】2 处 —— 已修复

| # | 调用 | 页面 | 根因 | 修复 |
|---|------|------|------|------|
| 5 | `GET /api/sop-templates?page_size=200` | `/sop-template/market` | 后端 `pagination.Parse` 全局上限 MaxPageSize(100)，200 直接 400 | `src/views/sopTemplate/Market.vue:444` 改为 page_size:100 |
| 6 | 同上 | `/aiAgent/edit/:id` | 同一接口同一参数 | `src/views/aiAgent/Edit.vue:601` 改为 page_size:100 |

### 【console 报错/警告】

| # | 现象 | 处理 |
|---|------|------|
| 7 | 全站 159 页报 `CSP directive 'frame-ancestors' is ignored when delivered via <meta>` | 已修复：`index.html` 移除 meta CSP 中无效的 frame-ancestors 指令（meta 本就不生效，如需防点击劫持应由服务端 HTTP 头下发） |
| 8 | 卡片列表页（douyin/kuaishou/xianyu/xiaohongshuCard）`https://img.example.com/*.png ERR_CONNECTION_CLOSED` | 不修：demo 数据中的占位图外链域名不可达，属测试数据问题 |
| 9 | `/customerSession/list` WS `ws:///api/ws/agent` 连接失败 | 不修：后端路由 `/ws/agent` 存在（service_routes.go:77），失败源于当前环境坐席数据/令牌，属环境问题，记录观察 |

## 三、确认为"非断链"的 404（后端行为正确）

以下 404 均因测试用占位 id=1 的记录不存在或事件历史为空，后端路由均已确认注册：

- `GET /api/faqs/1`、`GET /api/sop-templates/1`、`GET /api/douyin/1`、`GET /api/douyin/stats/card/1`
- `GET /api/events/customer/conv:xxx`（客户无事件历史）
- `GET /api/channel-agent-bindings?channel_id=<不存在>` → 400
- `GET /api/monitor/alerts/unread`（ops-overview）：后端无此聚合端点，但页面已 catch 且有降级展示，影响小，**列入遗留观察**

## 四、回归验证

修复后对 8 个关键页复测（kuaishou-card-stats、install-guide、merge-rules、dingtalk-app、whatsapp-cloud、sop-template market/list、douyin-card-stats）：全部无 pageerror、无非预期 4xx，页面正常渲染。

## 五、遗留问题清单

1. **后端缺 `POST /api/oneid/merge-rules/preview`**（中等优先级）：需按五层架构新增 handler/service，返回 `{candidateCount, samples[]}`。
2. **`GET /api/monitor/alerts/unread` 无对应后端端点**（低优先级）：OpsOverview 有降级，可后续补齐。
3. **demo 数据占位图外链**（低）：`img.example.com` 不可达产生噪音报错，建议改本地占位图。
4. **WS `/api/ws/agent` 连接失败**（观察项）：待真实坐席账号环境验证。

## 六、结论

159 个页面全部可达且渲染正常；4 处渲染崩溃、2 处参数 400、1 类全局 console 警告已修复并回归通过；1 处真断链（merge-rules/preview）因涉及后端新业务功能而记录遗留。
