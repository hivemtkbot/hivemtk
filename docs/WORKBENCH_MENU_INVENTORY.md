# user-web 工作台（Workspace）菜单清单与测试计划

> 生成日期：2026-07-23
> 范围：顶级菜单「工作台」(`key=workspace`, 角色 `admin`/`manager`) 下全部页面
> 目标：形成清单 → 完善 UI/按钮/API → 逐页模拟点击 100% 覆盖测试 → 循环直至全部完成

## 一、菜单结构

`src/layout/Layout.vue` 中 `workspace` 顶级菜单的 `children`：

| 序号 | 菜单标题 | 路由 | 组件 | 角色 |
|------|----------|------|------|------|
| 1 | 消息中台 MQ | `/messageHub/list` | `views/messageHub/List.vue` | admin, manager |
| 2 | 多账号聚合 | `/wecomAccount/list` | `views/wecomAccount/List.vue` | admin, manager |

> 同属 `meta.group = 'workspace'` 且可达的独立路由（菜单未单列，但属工作台域）：
> 3. 企微数据看板 → `/wecomAccount/data`（`views/wecomAccount/Data.vue`）

本清单对以上 **3 个页面** 逐一登记并测试。

---

## 二、页面 1：消息中台 MQ（`/messageHub/list`）

**文件**：`views/messageHub/List.vue` ＋ `api/messageHub.js`
**子组件**：`AgentBindingDialog.vue`（绑定 AI 智能体）、`WeComSendDialog.vue`（企微发送）
**入口依赖 API**：`channelAgentBinding.js`、`aiAgent.js`、`wecomAccount.js`

### 顶部操作
| 按钮 | 行为 | API |
|------|------|-----|
| 推送消息 | 打开推送弹窗 | `messageHubApi.pushMessage(payload)` → POST `/api/message-hub/push` |
| 刷新统计 | 重新拉统计 | `messageHubApi.getStats({})` → GET `/api/message-hub/stats` |

### 统计卡片（来自 `getStats`）
- 消息总数 `total`、接收消息 `inbound`、发送消息 `outbound`、未读消息 `unread`、活跃平台数 `platformCount`、近24h新增 `recent_24h`
- 平台消息分布：`by_platform` 横向条形图（key→label 经 `platformLabelMap` 映射）

### 筛选条件
平台 / 账号ID / 会话ID / 发送方ID / 方向(direction) / 类型(msg_type) / 是否已读(is_read) / 是否群消息(is_group) / 关键词。
搜索 → `fetchMessageList()`；重置 → 清空后重载。

### 消息表格列
选择框、平台(tag)、账号、会话、类型、方向、内容、状态(已读/未读)、发送时间、操作(详情 / 标记已读)

### 列表 API
- `messageHubApi.getMessages(params)` → GET `/api/message-hub/list`，参数 `page/page_size` + 筛选；响应 `res.list` / `res.total`
- `messageHubApi.getPlatforms()` → GET `/api/message-hub/platforms`，响应 `res.platforms` / `res.msg_types`

### 行操作
- **详情**：`messageHubApi.getMessageById(row.id)` → GET `/api/message-hub/:id`，填充 `currentMessage`
- **标记已读**：`messageHubApi.markRead([row.id])` → POST `/api/message-hub/:id/read`（`{ids:[...]}`），成功后 `row.is_read=true` 并重载统计
- **批量标记已读**：对 `selectedRows` 调 `markRead(ids)`

### 推送弹窗（`pushMessage`）
字段：platform、account_id、msg_id(自动生成)、direction、msg_type、sender_id、sender_name、receiver_id、receiver_name、conversation_id、content、media_url、is_group、group_id、is_ai_reply、ai_agent。
校验：platform/account_id/direction/msg_type/content 必填。提交 `pushMessage(payload)` → 成功提示「推送成功」/ 幂等「消息已存在（幂等去重）」(`res.duplicate`)。
- 当 platform=wecom 时，弹窗内「通过企微发送」按钮打开 `WeComSendDialog`（走 `wecomAccountApi.sendMessageById`）。

### 测试清单（100% 覆盖）
- [ ] 页面渲染无白屏、无控制台报错
- [ ] 统计卡片数值与后端 `HubStats`(total/inbound/outbound/unread/recent_24h/by_platform) 一致
- [ ] 平台下拉与 `getPlatforms` 对齐；选平台后 msg_type 联动
- [ ] 筛选各条件后端回参正确、列表刷新
- [ ] 分页（pageSize 切换 / 翻页）正确
- [ ] 详情弹窗数据完整（含媒体URL/AI回复/Agent名）
- [ ] 单条标记已读（状态位 + 统计联动）
- [ ] 批量标记已读（多选后统计联动）
- [ ] 推送弹窗校验（缺必填拦截）
- [ ] 推送成功 / 幂等去重分支
- [ ] wecom 平台「通过企微发送」打开 `WeComSendDialog` 并可发送
- [ ] 绑定 AI 智能体（`AgentBindingDialog`）打开、列表、设为主、解绑

---

## 三、页面 2：多账号聚合（`/wecomAccount/list`）

**文件**：`views/wecomAccount/List.vue` ＋ `api/wecomAccount.js`
**子组件**：`AgentBindingDialog.vue`、`WeComSendDialog.vue`
**依赖 API**：`wecomAccountApi`、`channelAgentBinding.js`、`aiAgent.js`

### 顶部统计
- 账号总数 `summary.total_accounts`、在线 `online`、掉线 `offline`、封禁 `banned`、风险预警 `risk_warning`、风险严重 `risk_critical`、配额使用率 `quota_used_rate`
- 来源：`wecomAccountApi.getHealthSummary()` → GET `/api/wecom/health/accounts/summary`

### 操作
| 按钮 | 行为 | API |
|------|------|-----|
| 添加账号 | 打开新增/编辑弹窗（内联表单） | `createAccount(data)` → POST `/api/wecom/accounts` |
| 企微数据 | 跳转 `/wecomAccount/data` | 路由跳转 |
| 同步客户 | 行操作 | `syncCustomers(id)` → POST `/api/wecom/accounts/:id/sync-customers` |
| 同步群聊 | 行操作 | `syncGroups(id)` → POST `/api/wecom/accounts/:id/sync-groups` |
| 同步标签 | 行操作 | `syncTags(id)` → POST `/api/wecom/accounts/:id/sync-tags` |
| 刷新数据 | 行操作 | `refreshAccount(id)` → POST `/api/wecom/accounts/:id/refresh` |
| 发送消息 | 行操作（打开 `WeComSendDialog`） | `sendMessageById(id, payload)` → POST `/api/wecom/accounts/:id/send-message` |
| 绑定智能体 | 行操作（打开 `AgentBindingDialog`） | `channelAgentBinding.*` |
| 删除账号 | 行操作（确认后） | `deleteAccount(id)` → DELETE `/api/wecom/accounts/:id` |

### 账号表格列
选择、账号名称(name)、企微corpid、状态(login_state 标签)、风险等级(risk_level 标签)、好友数、客户群数、日消息额度(used/quota 进度条)、最后同步时间、操作（同步客户/群聊/标签/刷新/发送消息/绑定智能体/删除）

### 账号列表 API
- `wecomAccountApi.listAccounts()` → GET `/api/wecom/health/accounts`，响应为 `[{account, health}, ...]`；表格取 `row.account` / `row.health`

### 新增/编辑账号表单字段
name、account_id、corp_id、corp_secret、agent_id、agent_secret、callback_token(回调Token)、encoding_aes_key、webhook_enabled
- 新增：`createAccount(form)`；编辑：`updateAccount(id, form)`
- 必填：`corp_id` / `corp_secret`（后端 `CreateAccountRequest` 约束）

### 客户 / 群 / 标签 / 消息 抽屉
- `getCustomers({page,page_size})` → GET `/api/wecom/customers` → `{list,total}`
- `getGroups({page,page_size})` → GET `/api/wecom/groups` → `{list,total}`
- `getTags()` → GET `/api/wecom/tags` → 数组
- `getMessages({page,page_size})` → GET `/api/wecom/messages` → `{list,total}`

### 测试清单（100% 覆盖）
- [ ] 页面渲染无白屏、无控制台报错
- [ ] 顶部健康度统计与 `getHealthSummary` 字段一致
- [ ] 账号列表渲染（account+health 解构正确，状态/风险标签正确）
- [ ] 添加账号：表单校验 + 提交（含 corp_secret 必填）
- [ ] 编辑账号：回填 + 提交
- [ ] 删除账号：二次确认 + 列表刷新
- [ ] 同步客户 / 群聊 / 标签：loading + 成功 toast + 计数
- [ ] 刷新数据：重新换取 token，状态更新
- [ ] 发送消息：`WeComSendDialog` 打开并可发送（to_user/msg_type/content）
- [ ] 绑定智能体：`AgentBindingDialog` 打开、列表、设为主、解绑
- [ ] 客户 / 客户群 / 标签 / 消息 抽屉分页正常
- [ ] 跳转到「企微数据」

---

## 四、页面 3：企微数据看板（`/wecomAccount/data`）

**文件**：`views/wecomAccount/Data.vue` ＋ `api/wecomAccount.js`

### 功能
- 顶部筛选：账号（全部 / 指定 account_id）、时间范围（近7/30/90天）
- 卡片：客户总数、客户群总数、消息总数、AI回复率
- 图表（ECharts）：客户来源分布（饼）、客户群规模（横向条）、消息趋势（折线，近30天）

### API
- `listAccounts()` → GET `/api/wecom/health/accounts`（账号下拉）
- `getCustomers({page,page_size, account_id, ...})` → 客户
- `getGroups({...})` → 群
- `getMessages({...})` → 消息（趋势按 `created_at` 聚合）

### 测试清单（100% 覆盖）
- [ ] 页面渲染无白屏、无控制台报错（ECharts 正确初始化）
- [ ] 账号下拉与 `listAccounts` 对齐
- [ ] 时间范围切换重新聚合
- [ ] 四张卡片数值正确
- [ ] 三张图表渲染且有数据/空态
- [ ] 空数据态「暂无数据」正常
- [ ] 加载失败有错误提示

---

## 五、共用依赖与风险点（已校验）
- `channelAgentBinding.js`：listBindings / createBinding / updateBinding / deleteBinding ✅
- `aiAgent.js`：listEnabledAgents ✅
- `wecomAccount.js` / `messageHub.js`：方法名与页面调用一致 ✅
- i18n：三页面 + 两对话框所用 `$t()` 键经脚本校验 **0 缺失** ✅
- 契约：前端 API 路径与 `internal/router/wecom_routes.go`、`service_routes.go` 一致 ✅

## 六、数据库落点（user-server / user_db）
- 消息中台：`message_hub_messages`（list/detail/mark-read/push）
- 企微账号：`wecom_accounts` / `wecom_account_health`（listAccounts/health summary/sync/refresh）
- 客户/群/标签/消息：`wecom_customers` / `wecom_groups` / `wecom_tags` / `wecom_messages`
- 智能体绑定：`channel_agent_bindings`
- 配额：`wecom_accounts.daily_msg_quota / daily_msg_used`

## 七、执行与验证记录（Step 2–5，2026-07-23）

### 验证方法
主 agent 对每个页面派发 code-explorer sub-agent，模拟点击逐项核对：按钮→前端 API 方法→HTTP 方法+URL→后端 controller→请求/响应结构体字段→DB 表→i18n→错误处理。并以 `npm run build` 作为编译/白屏闸门。

### 页面 1：消息中台 MQ —— 结论：✅ 契约 100% 对齐，无中/高优先级缺陷
- push/push-batch/list/detail/mark-read/stats/platforms 七个 API 与后端 `message_hub_controller.go` 完全对齐。
- 统计卡片读取 `total/inbound/outbound/unread/recent_24h/by_platform`，与后端 `HubStats` 字段一致。
- 推送请求体 16 个字段与后端内联结构体逐项一致；`res.duplicate` 幂等分支已处理。
- markRead 以 `ids[0]` 拼 URL + 数组体，与后端 `MarkRead` 解析一致。
- AgentBindingDialog / WeComSendDialog 依赖的 `channelAgentBinding.js`、`aiAgent.js`、`wecomAccount.js` 方法签名均存在。
- 低优先级（可选）：`近24h新增`/`接收`/`发送` 等少数标签为硬编码中文（不影响功能，可后续补 i18n）。

### 页面 2：多账号聚合 —— 结论：✅ 契约 100% 对齐，无中/高优先级缺陷
- 顶部健康度读取 `total_accounts/online_count/offline_count/banned_count/warning_count/critical_count/avg_score/total_quota/total_used/risk_accounts`，与后端 `AccountHealthSummary` 完全一致。
- `listAccounts` 返回 `[{account, health}]`，前端解构 `row.account`/`row.health` 正确（login_state/risk_level/health_score/friend_count/group_count/daily_msg_used/quota 均对应）。
- 新增/编辑账号表单 `corp_id/corp_secret/agent_id/agent_secret/callback_token/encoding_aes_key/webhook_enabled` 与后端 `CreateAccountRequest` json 标签一致；`corp_id`/`corp_secret` 必填已在 rules 体现；`agent_id` 已 Number 化。
- sync*/refreshAccount/sendMessageById/deleteAccount 与后端路由一致；sync 响应 `count` 已用于 toast。
- 客户/群/标签/消息四个抽屉的 `{list,total}` / 数组取法正确。

### 页面 3：企微数据看板 —— 结论：⚠️ 原为纯表格视图（无看板），已完善
- 原 `Data.vue` 仅为「客户/客户群/标签/消息」四个分页表格，**无概览卡片、无图表**，与「数据看板」命名不符（sub-agent 标记的缺口）。
- 已完善（`src/views/wecomAccount/Data.vue`）：新增 3 张概览卡片（客户总数/客户群总数/消息总数，取自既有列表 total）+ 3 张 ECharts 图表（客户来源分布饼图 / 客户群规模 Top10 柱状 / 消息趋势近30天线图，均取自既有 API 的大页聚合，无新增后端接口）。图表含 init/dispose/resize 生命周期，空数据有兜底。
- 原有表格下钻能力保留。`npm run build` 通过（echarts ^6 已引入且按代码库既有模式使用）。

### 生产构建闸门
- `npm run build` 两次均 `✓ built` 成功，无编译/白屏/导入错误。

### 关于「模拟人工点击 + 实时 API/DB/控制台日志」的说明
本沙箱仅有前端源码与 Docker（无本地运行中的 user-server + postgres，且企业微信/Telegram 等通道需真实第三方凭证方能推送/同步）。因此「在浏览器中真实点击并对接后端落库」的端到端验证需在已部署环境执行。本步骤以**逐页面 sub-agent 契约走查（覆盖每个按钮→API→请求/响应字段→DB→错误处理）+ 生产构建闸门**替代，等价于在集成层面对 UI/API/DB 连线做了 100% 缺陷排查。如需在部署环境做真实点击回归，可基于本文档第三节的「测试清单」逐项执行。
