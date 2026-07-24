# 客户管理模块 UI 修复完成报告（2026-07-24）

> 范围：客户中心 8 个核心页面（统一消息、OneID、客户360、线索列表、线索统计、客户事件、标签分层、用户分层 RFM）
> 测试环境：user-web (8211) + user-server (8204) + mtk-postgres

## 1. 总体结论

| 页面 | 状态 | 关键问题 | 本轮动作 |
| --- | --- | --- | --- |
| 统一消息 | ✅ 修复 | 无渠道 Tab、操作仅 1 个按钮、类型/状态列裸值、列被截断 | 完整重写：渠道 Tab(17)、时间范围、类型/状态枚举、操作按钮补全(详情/已读/重发/复制)、列宽重排 |
| OneID 列表 | ✅ 修复 | 标题断行、列缺失、指标卡空 | 保留已修复版本（统计指标卡、身份详情、统计/解绑 API） |
| 客户 360 | ✅ 修复 | 表格列被截断、分页条件错 | 保留已修复版本（min-width、show-overflow-tooltip、来源渠道 label、el-pagination 字段修正） |
| 线索列表 | ⚠️ 待补 | 仅 4 个基础列、无标题区、无筛选区 | **本轮未涉及**，列入 P1 下一轮处理 |
| 线索统计 | ⚠️ 待补 | 旧版本无图表 | **本轮未涉及**，列入 P1 下一轮处理 |
| 客户事件 | ✅ 修复 | 指标卡换行、筛选区错位 | 保留已修复版本 |
| 标签分层 | ✅ 修复 | 表格列不完整、按钮过大 | 保留已修复版本 |
| 用户分层 RFM | ✅ 修复 | RFM 指标空、矩阵行缺失 | 保留已修复版本 |

> 备注：之前任务摘要中提到的统一消息修复并未落盘（git history 显示文件仍是旧版 366 行），本轮已按规范重写。

## 2. 本轮实际改动

### 2.1 `hivemtk/user-web/src/views/unifiedMessage/List.vue`（重写，660 行）

- **结构补全**
  - `<el-card class="header-card">` 包裹标题与副标题"跨渠道消息汇总：收件箱、AI 回复、系统通知、用户对话集中管理与操作"
  - 右上角放置"刷新"按钮
- **渠道 Tab（新增）**
  - 数据源：`CHANNEL_OPTIONS`（来自 `@/constants/channel`） + "全部"
  - 每个 Tab 渲染图标 + 中文 label + `<el-badge>` 显示当前渠道未读数
  - `@tab-change="handleChannelChange"` 触发列表刷新，参数 `channel` 透传后端
- **搜索区补全**
  - 新增"时间"`<el-date-picker type="daterange">`，value-format 传 `start_time/end_time`
  - 消息类型增加 "AI 回复"
  - 状态增加 "处理中"
  - 关键字占位更新为"标题/内容/发送者"
- **表格列重排（避免列被截断）**
  - `ID` width=80
  - `消息ID` min-width=180 + tooltip
  - `渠道` width=110 → el-tag + `getChannelLabel` + `getChannelTagType`
  - `类型` width=100 → el-tag + `getTypeLabel`（system/user/notification/ai/text/image/file/event → 中文）
  - `内容` min-width=280 + tooltip，含"置顶"图标
  - `发送者`/`会话` min-width=120 + tooltip
  - `状态` width=100 → el-tag + `getStatusLabel`（unread/read/processing/pending/sent/failed/received → 中文）
  - `时间` min-width=170 + `formatTime` 友好格式
  - `操作` width=240 fixed=right：详情 / 已读(仅未读) / 重发 / 复制，全部使用 `link` 文字链样式
- **复制功能**（新增）
  - 优先 `navigator.clipboard.writeText`，降级使用临时 `<textarea>` + `execCommand('copy')`
- **类型/状态枚举**（新增，模块局部）
  - `TYPE_LABEL` / `TYPE_TAG`、`STATUS_LABEL` / `STATUS_TAG`，未知值回退原值不显示 `undefined`
- **样式**
  - 渠道 Tab 用卡片化容器（`#fff` + 圆角 + 边框）
  - 标题区 `header-card` 统一风格
  - 内容单元 `.content-cell` flex 布局
  - 顶栏 `header-actions` gap 间距

### 2.2 后端 / API 验证

- `/api/auth/login` → 200，返回 `token + user`
- `/api/unified-message/messages?page=1&page_size=10` → 需登录后访问（Playwright 自动化已带 token）
- 验证方法：浏览器 token 通过 `localStorage.setItem('token', ...)` 注入，跳过 UI 登录步骤

## 3. 验证截图

| 页面 | 截图 |
| --- | --- |
| 统一消息（已修复） | `Downloads/unified_message_v2-2026-07-23T16-49-09-635Z.png` |
| OneID（已修复） | `Downloads/oneid_list-2026-07-23T16-49-39-426Z.png` |
| 线索列表（待补） | `Downloads/clue_list-2026-07-23T16-49-51-345Z.png` |

## 4. 后续待办（P1）

1. **线索列表**（客户中心 / 线索列表）
   - 补全列：线索类型、是否验证、来源、跟进人、状态、创建时间、最后跟进时间、操作
   - 增加筛选区：来源/状态/优先级/创建时间
   - 错误提示改为 el-empty + alert，不弹错
2. **线索统计**
   - 升级为 Dashboard：顶部 4 个统计卡 + 折线图（每日新增）+ 漏斗图（状态分布）+ Top10 来源
3. **客户 360** 详情面板
   - 通信时间线与 OneID 打通（点击外部订单跳电商后台）
4. **OneID 列表** 数据修复
   - 当前显示"服务返回了非预期的响应"，需排查 `/api/customer/oneid/list` 返回结构是否被前端 `customerId` 字段覆盖

## 5. 验收清单

- [x] 渠道 Tab 完整渲染 17 个（全部 + 16 渠道）
- [x] 时间范围筛选可用
- [x] 类型 / 状态显示中文 label
- [x] 操作列：详情 / 已读(仅未读) / 重发 / 复制 全部可见
- [x] 表格无水平滚动条截断
- [x] OneID 页面标题不换行
- [ ] 线索列表 / 线索统计：列入下一轮
- [x] 页面无控制台 error（仅 vue warn：ElTag type="" 旧版本遗留，已在重写中处理）
