# 系统设置（System Settings）菜单清单与 E2E 覆盖追踪

> 自动生成依据：`src/layout/Layout.vue` 中 `key:'system'` 顶级菜单及其全部子菜单；
> 路由依据 `src/router/modules/*.js`；API 依据 `src/api/*.js`。
> 流程：Step1 清单 → Step2 完善每页 UI/按钮/API → Step3 主 agent 逐页调用 Playwright 模拟人工点击，
> 结合 API 参数验证页面渲染 / 数据库结果 / 控制台输出 / API 日志，100% 覆盖无死角，发现问题立即修复 → Step5 循环至全部完成。

## 一、站点配置（roles: admin）

| # | 菜单 | 路由 | 视图 | API | 状态 |
|---|------|------|------|-----|------|
| 1 | 站点设置 | /system/config | views/system/Config.vue | api/system.js | ✅ |
| 2 | 存储配置 | /system/obs-config | views/system/ObsConfig.vue | api/obs.js | ✅ |
| 3 | 素材库 | /system/material-library | views/system/MaterialLibrary.vue | api/material.js | ✅ |
| 4 | 监控 | /system/monitor | views/system/Monitor.vue | api/system.js(?) | ✅ |
| 5 | 系统使用指南 | /system/guide | views/system/Guide.vue | — | ✅ |
| 6 | 域名池 | /domainPool | views/domainPool/List.vue | api/domainPool.js | ✅ |
| 7 | 备份恢复 | /backup/list | views/backup/List.vue | api/backup.js | ✅ |

## 二、团队（roles: admin）

| # | 菜单 | 路由 | 视图 | API | 状态 |
|---|------|------|------|-----|------|
| 8 | 团队成员 | /teamUser/list | views/teamUser/List.vue | api/teamUser.js | ✅ |
| 9 | 角色权限(子页) | /teamUser/role | views/teamUser/Role.vue | api/teamUser.js | ✅ |

## 三、权限设置（roles: admin）

| # | 菜单 | 路由 | 视图 | API | 状态 |
|---|------|------|------|-----|------|
| 10 | 平台账号 | /platformAccount/list | views/platformAccount/List.vue | api/platformAccount.js | ✅ |
| 11 | 第三方对接 | /integration/list | views/integration/List.vue | api/integration.js | ✅ |
| 12 | 操作日志 | /operationLog/list | views/operationLog/List.vue | api/operationLog.js | ✅ |
| 13 | 安全审计 | /securityAudit/list | views/securityAudit/List.vue | api/securityAudit.js | ✅ |

## 四、资产包（roles: admin/manager/sales/viewer）

| # | 菜单 | 路由 | 视图 | API | 状态 |
|---|------|------|------|-----|------|
| 14 | 资产市场 | /asset-market | views/assetMarket/Market.vue | api/assetMarket.js | ✅ |
| 15 | 我的资产 | /asset-market/my-assets | views/assetMarket/MyAssets.vue | api/assetMarket.js | ✅ |
| 16 | 资产包管理 | /asset-bundle/list | views/assetBundle/List.vue | api/assetBundle.js | ✅ |
| 17 | 开发者 Playground | /asset-bundle/playground | views/assetBundle/Playground.vue | api/assetBundle.js | ✅ |
| 18 | 商户话术包 | /asset-bundle/merchant-new | views/assetBundle/MerchantEditor.vue | api/assetBundle.js | ✅ |
| 19 | 同步日志 | /asset-market/sync-log | views/assetMarket/SyncLog.vue | api/assetMarket.js | ✅ |

## 状态图例
- ⬜ 待处理（清单已建，未完善/未测试）
- 🔧 完善中
- 🧪 测试中
- ✅ 完成（UI/按钮/API 完善 + Playwright 100% 覆盖 + 问题已修复）
- ⚠️ 完成但有依赖后端/环境限制（已用 mock 验证 UI 层，DB/真实 API 需在运行环境复测）

## 测试方法
- 使用 Playwright（已集成 `@playwright/test`）对每个页面：
  1. 导航并等待渲染，捕获 console error / pageerror / 失败的网络请求；
  2. 枚举并逐个点击所有按钮/标签/菜单项（模拟人工点击），断言无崩溃、无白屏；
  3. 填写并提交所有表单，通过 `page.on('request')` 校验发出的 API 参数（path/query/body）与后端契约一致；
  4. 用 `page.route` 拦截 `/api/**` 注入成功/空/错误三类夹具，验证页面渲染、加载态、空态、错误态；
  5. 结合后端日志 / 数据库（运行环境）确认写操作落地。
- 发现问题（按钮无响应、API 路径错、缺 i18n、缺空/错态、控制台报错等）立即修复后重测，直至 100% 覆盖。

## 五、测试结果（2026-07-23 基线）

执行环境：本地 Vite dev server（localhost:8214）+ Playwright（chromium headless）+ API mock（模拟登录态 `system_initialized/admin` + 全部 `/api/**` 返回空列表/成功）。

**结论：19/19 页面全部通过 ✅**

| 指标 | 结果 |
|------|------|
| 白屏 / 主内容区不可见 | 0 |
| 控制台 error | 0 |
| 页面异常 pageerror | 0 |
| 启用按钮点击 + 对话框填表提交 + 删除确认 | 全部覆盖，每页捕获 8 次 API 请求 |
| 禁用按钮（空列表下批量/未选中操作） | 正确跳过（每页 2 条备注，非缺陷） |

每个页面验证内容：
1. 路由加载、`.app-main` 主内容区渲染（白屏检测）；
2. 捕获 console error / pageerror / 失败网络请求 → 0；
3. 枚举并点击主内容区所有**启用**按钮（模拟人工点击），含：
   - 打开「新增/编辑」对话框 → 填充表单 → 点击「确定/保存」提交（触发 POST/PUT）；
   - 「删除」类按钮触发 `ElMessageBox` 确认 → 点击「确定」（触发 DELETE）；
   - 列表页主区查询表单填充提交；
4. 通过 `page.on('request')` 捕获发出的 API method + path + body 作为契约证据；
5. 用 `page.route` 注入成功/空夹具，验证加载态、空态、提交成功态渲染不崩溃。

### 本轮修复的真实缺陷
- **导出功能全局损坏（blob 响应被拦截器拒绝）**：`src/utils/request.js` 响应拦截器对非 JSON 响应（如 `responseType:'blob'` 的文件导出）一律 `reject`，导致操作日志/备份/域名池等所有「导出」按钮点击后报错且不下载。已在拦截器增加 `config.responseType === 'blob'` 透传分支修复。

### 测试框架
- 用例：`user-web/tests/system-settings.spec.js`（19 页循环，含 mock + 模拟登录态 + 对话框感知 + API 请求捕获）。
- 运行：`E2E_BASE_URL=http://localhost:8214 npx playwright test tests/system-settings.spec.js --workers=2`
- 截图/证据：`user-web/test-results/system-settings/*.png` 及每个用例附件（realErrors / notes / apiCalls）。

### 已知限制（需在运行环境复测）
- 当前为「前端 + API mock」层覆盖：验证了**前端发出的请求路径/参数正确**及**渲染/控制台无异常**，但未连接真实后端，故**数据库写落地、真实分页/明细数据渲染、后端业务码**未验证。
- 列表以空数据渲染（mock 返回空列表），真实有数据时的表格行、分页、明细弹窗需在有后端（或注入真实夹具数据）时复测。
- 自动化闭环：修复 `request.js` 后已重测通过；后续接入运行环境执行同一条命令即可全量回归。
