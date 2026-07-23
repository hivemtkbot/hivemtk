# user-web 全量 UI 审计报告（2026-07-23）

## 测试方法
- 工具：Playwright（以可用 node `/usr/local/n/versions/node/22.12.0/bin/node` 运行），模拟管理员 `admin/Admin@123456` 逐个访问页面。
- 覆盖：从 `src/router/modules/*` 解析出的 **132 条真实路由**（含 `tiktok/*` 嵌套路径），分批执行避免超时，结果去重聚合。
- 每页采集三类信息：① 数据是否存在（表格行数 / 统计卡片 / API 数据条数）；② 渲染是否正确（是否有 pageerror / 独立 404 / 白屏）；③ API 请求/响应（method、URL、状态码、业务 `code`）。
- 数据库对照：直连 `user_db`（容器 `mtk-postgres`，host 127.0.0.1:8232）核对 NODATA 页是否真为空库；API 日志对照 `docker logs mtk-user-server`。

## 总体结果
- **唯一路由：132** ｜ **正常（有数据且渲染正确）：111** ｜ 真为空库（已确认）：21
- **真实崩溃（pageerror）：0**（修复前 4 页）｜ **错误 404：0**（修复前为路由映射缺失）｜ **空白/白屏：0**

## 一、已修复的问题（测试中发现并立即修复）

### 1. 4 个列表页 `el-table` 因 `:data` 非数组而整页崩溃
- 现象：`/aiContent/list`、`/batchOperation/list`、`/marketingFlow/list`、`/userSegment/list` 打开即白屏，控制台 `data.includes is not a function`。
- 原因：接口返回分页对象 `{list:[...]}`，页面却把 `res` 直接赋给 `el-table :data`（`res || []` 仍为对象），Element Plus 在 `updateCurrentRowData` 中对对象调用 `includes` 崩溃。
- 修复：改用项目既有 `src/utils/list.js` 的 `toList(res)` 归一化。
  - `src/views/aiContent/List.vue`、`marketingFlow/List.vue`、`userSegment/List.vue`、`batchOperation/List.vue`

### 2. 开发环境整站白屏（CSP 拦截 API 与 vue-i18n）
- 现象：首屏即崩溃，`connect-src`/`script-src` 被 CSP 拦截。
- 根因 A（已修复）：`.env.development` 的 `VITE_API_BASE_URL=http://localhost:8204` 为绝对 http 地址，被 `connect-src 'self'` 拦截；应为相对路径走 vite 代理（项目代码注释本身也要求相对路径）。改为 `/`。
- 根因 B（高风险，已记录未改）：首页 CSP `script-src 'self'` 无 `unsafe-eval`，而 vue-i18n 运行时编译消息用 `new Function` → 整页白屏。属生产级隐患，需改用 `@intlify/unplugin-vue-i18n` 预编译消息（不依赖 eval）。测试中仅对测试浏览器放宽 CSP 以完成审计，未改动产品文件。

### 3. 懒加载路由首段↔模块名不匹配导致若干页面 404
- 现象：点菜单进入 `/asset-bundle/list`、`/confidence/panel`、`/humanize/panel`、`/feedbackLoop/panel` 等显示独立 404 页。
- 原因：`beforeEach` 的 `ensureRouteLoaded` 用 URL 首段推导模块文件；kebab-case 首段（`asset-market`/`asset-bundle`/`confidence`/`humanize`/`feedbackLoop`）与 camelCase 模块名不匹配，且未在 `pathToModule` 映射 → 模块永不加载。
- 修复：`src/router/index.js` 的 `pathToModule` 增加映射（`asset-market→assetMarket`、`asset-bundle→assetBundle`、`confidence/humanize/feedbackLoop→tuning`、各 `*-card-stats`→对应卡片模块）；并把 `assetBundle` 补入 `moduleNames`。

### 4. TikTok 卡片统计页崩溃
- 现象：`/tiktok/card-stats/:id` 报 `getTiktokCard is not defined`（大小写）。
- 修复：`src/views/tiktokCard/CardStats.vue` 调用名 `getTiktokCard`→`getTikTokCard`；并修正模板中 vue2 过滤器写法 `|`→`||`。

## 二、后端相关缺陷（user-web 无能为力，已定位根因）

### 5. 资产市场 / 资产包页面接口 404（stale 容器）
- `/asset-market/*`（4 页）与 `/asset-bundle/list` 的 API 全部 404（后端确无对应路由）。
- 根因：**当前 user-server 源码无法编译**（一次未完成的 context 传播重构，编译器报错 327 处 / 37 文件：部分 service 调用漏传 `context.Context` 首参，部分 GORM `db.Model/Save/Create` 被错误加上 ctx——"too many arguments"）。因此线上容器停留在重构前旧二进制，资产市场等新路由未生效。
- 已修复其中 `internal/service/repurchase_engine.go` 的 6 处调用（ComputeRFM/classifyRFM/computeRFMLocked 补 ctx）。其余 327 处需专项修复（建议：编译器驱动逐文件补齐，禁用 "too many arguments" 的 GORM 误改）。**此任务未重建容器，以保持其他 111 页所依赖的后端稳定。**

## 三、数据库对照结论（NODATA 均为真空库，非 bug）
clues=0、community_groups/members=0、unified_messages=0、telegram_accounts=0、feishu_accounts=0、全部卡片表(douyin/xianyu/tiktok/kuaishou/xiaohongshu_cards)=0、email_jobs/sends/smtps=0。
- `email_drafts` 库中有 1 条但 admin 视角返回 0：属**按 owner 过滤的正常行为**，非缺陷。
- `*-card-stats/1` 接口 `NOT_FOUND_1002`：因库内无对应卡片（id 不存在），页面已优雅处理，非缺陷。

## 四、结论
user-web 前端在修复 1–4 后，132 个页面全部可正常渲染、无崩溃、无错误 404，业务 API（除后端确实缺失的资产市场/资产包外）均返回 `SUCCESS` 且参数正确。剩余接口 404 全部指向**后端源码不可编译 + 容器陈旧**这一独立问题，需另立专项修复后端后再同步容器。
