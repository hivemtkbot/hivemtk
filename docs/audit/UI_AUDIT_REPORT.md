# user-web 全量页面 UI 审计

> 适用范围：`hivemtk/user-web`（Vue 2 + Element UI）。
> 审计内容：菜单路径（路由）、页面 UI 规格、每个元素的详细清单（筛选控件 / 表格列 / 表单字段 / 按钮 / 弹窗 / 图表）。
> 菜单结构以 `src/layout/Layout.vue` 的 `topMenus` 为准（工作台 / 客户中心 / 智能体 / 触达运营 / 知识中心 / 数据分析 / 系统设置），外加独立页面（登录 / 初始化 / 个人中心 / 通知 / 嵌入窗 / 404 / 改密 / 找回密码）。

---

## 菜单审计：重复 / 废弃 / 不需要（2026-07-22 逐项核验）

> 方法：提取 `Layout.vue` 的 110 个菜单 path，逐一比对 `router/index.js` + `router/modules/*.js` 路由定义与 `src/views` 下 `.vue` 实体文件，交叉判定重复/占位/孤儿。**全部为只读核验，未改动业务代码。**

### 结论概览
| 维度 | 结果 |
|---|---|
| 路由缺失 / 视图文件不存在 | **0 项** —— 110 个菜单 path 全部命中路由，且视图文件真实存在 |
| 空壳/废弃页面（"开发中/TODO/敬请期待"或 <30 行） | **0 项** —— 无需删除的空壳文件 |
| 真正孤儿视图（无路由、无菜单、无 import） | **0 项** |
| 语义重复菜单 | 1 项（"使用引导"重名） |
| 功能桩（有 UI 但不接接口） | 1 项（OneID 冲突解决） |
| 死链风险 | 1 项（系统使用引导内跳转路径失效） |
| 有路由但无菜单入口（隐藏页，非污染） | 若干（见下，属正常设计） |

### 1. 需要处理项（真实问题，均非"删除菜单"）

- **[语义重复] 两个"使用引导"重名**
  - `触达运营 › 邮件触达 › 使用引导`（`emailGuide` → `/email/guide` → `views/email/Guide.vue`）
  - `系统设置 › 使用引导`（`systemGuide` → `/system/guide` → `views/system/Guide.vue`）
  - 两项标题完全相同，侧栏易混淆。**建议改名区分**：如"邮件使用指南" / "系统使用指南"。

- **[死链] `views/system/Guide.vue` 内跳转链接失效**
  - 内含 `/system/init`、`/system/material`、`/douyin-card/list`、`/rag-product-config` 等**已不存在的路径**（现行分别为 `/setup`、`/system/material-library`、`/douyinCard`、`/system/rag-product-config`）。点击会落 404。建议修正为现行 path。

- **[功能桩] `oneid/conflicts`（身份冲突解决）未接后端**
  - `views/oneid/Conflicts.vue` 的 `loadConflicts()` 注释"此处为占位，避免阻塞构建"，`conflicts.value = []` 恒空、不调接口。UI 完整但功能未通。**保留菜单**，待后端冲突接口接通后接线；短期可考虑隐藏该子菜单避免误导。

### 2. 无需处理项（澄清，避免误删）

- **抖音/快手/小红书/咸鱼/TikTok 各系列"卡片+自动回复+统计"三件套**：均有独立路由与视图，非重复，属各渠道独立功能，保留。
- **路由别名复用组件**（`aiAgent`↔`aiAgent/list`、`whatsapp`↔`whatsapp/account`、`feishu`↔`feishu/account`、`telegram`↔`telegram/account`、`aiAgent/create`↔`aiAgent/edit/:id` 共用 `Edit.vue`）：为合理别名/复用，菜单只引用其一，**非污染**。
- **有路由但无菜单入口（隐藏页，属正常）**：`tuning` 三面板（`confidence/humanize/feedbackLoop` Panel）、`whatsappBot`（LeadGroupSelection/BulkMessaging）、各卡片 `*-card-stats/:id` 详情、`chatChannel` 的 create/edit/install-guide、`teamUser/role`、`assetMarket` 的 detail/my-assets/sync-log 等 —— 由列表页跳转或为二级操作页，**不应作为顶层菜单**，无需清理。若确需暴露 `tuning`/`whatsappBot` 给用户，再补菜单配置。

### 3. 处置建议与状态（2026-07-22 已执行）

1. ✅ **[已修复] 改名消除"使用引导"重复**：`Layout.vue` 中邮件触达下 `emailGuide` 标题改为 `邮件使用指南`，系统设置下 `systemGuide` 标题改为 `系统使用指南`（与 `system/Guide.vue` 页头 `$t('系统使用指南')` 一致）。
2. ✅ **[已修复] 死链**：`views/system/Guide.vue` 内 4 处失效路径已修正 —— `/system/init`→`/setup`、`/system/material`→`/system/material-library`、`/douyin-card/list`→`/douyinCard`、`/rag-product-config`→`/system/rag-product-config`（页内"`/system/obs-config`"本就是现行路径，无需改）。
3. ✅ **[已处理] `oneid/conflicts` 桩逻辑**：接口未通前已从 `Layout.vue` 菜单隐藏（保留路由与 `Conflicts.vue` 组件，待后端 `/api/oneid/conflicts` 接通后恢复入口）。`loadConflicts()` 仍为空占位，暂无功能。
4. 其余菜单结构健康，**无重复菜单需删除、无废弃页面需清理**。

> 注：两处 `使用引导` 标题为纯文本（非 i18n key），改名未涉及 i18n 文案；`system/Guide.vue` 页头标题用了 `$t('系统使用指南')` 硬编码文案，与菜单一致。

---

## 总览：页面清单

| 一级菜单 | 路由 | 页面标题 | 组件文件 |
|---|---|---|---|
| 工作台 | /messageHub/list | 消息中台 MQ | views/messageHub/List.vue |
| 工作台 | /wecomAccount/list | 企微账号管理 | views/wecomAccount/List.vue |
| 客户中心 | /clue/list | 线索列表 | views/clue/List.vue |
| 客户中心 | /clue/statistics | 线索统计 | views/clue/Statistics.vue |
| 客户中心 | /customer360/list | 客户 360 | views/customer360/List.vue |
| 客户中心 | /customerEvent/list | 客户事件 | views/customerEvent/List.vue |
| 客户中心 | /tagSegmentation/list | 标签分群 | views/tagSegmentation/List.vue |
| 客户中心 | /userSegment/list | 用户分群 | views/userSegment/List.vue |
| 客户中心 | /unifiedMessage/list | 统一消息 | views/unifiedMessage/List.vue |
| 客户中心 | /order/list | 订单管理 | views/order/List.vue |
| 客户中心 | /oneid/list | OneID 合并 | views/oneid/List.vue |
| 客户中心 | /oneid/conflicts | OneID 冲突 | views/oneid/Conflicts.vue |
| 智能体 | /aiAgent/list | 智能体列表 | views/aiAgent/List.vue |
| 智能体 | /aiAgent/create | 创建智能体 | views/aiAgent/Edit.vue |
| 智能体 | /unifiedInbox/list | 统一收件箱 | views/unifiedInbox/List.vue |
| 智能体 | /customerSession/list | 客户会话 | views/customerSession/List.vue |
| 智能体 | /intentRecognition/list | 意图识别 | views/intentRecognition/List.vue |
| 智能体 | /dialogueMemory/list | 对话记忆 | views/dialogueMemory/List.vue |
| 智能体 | /llmRouting/list | LLM 路由 | views/llmRouting/List.vue |
| 智能体 | /reachPipeline/list | 触达流水线 | views/reachPipeline/List.vue |
| 智能体 | /marketingFlow/list | 营销流程 | views/marketingFlow/List.vue |
| 智能体 | /batchOperation/list | 批量操作 | views/batchOperation/List.vue |
| 智能体 | /sopAgent/list | SOP 智能体 | views/sopAgent/List.vue |
| 智能体 | /scriptTemplate/list | 话术模板 | views/scriptTemplate/List.vue |
| 智能体 | /customerService/agentStatus | 客服坐席状态 | views/customerService/AgentStatus.vue |
| 智能体 | /customerService/quickReply | 快捷回复 | views/customerService/QuickReply.vue |
| 智能体 | /customerService/sessionTag | 会话标签 | views/customerService/SessionTag.vue |
| 智能体 | /customerService/aiSuggestion | AI 辅助建议 | views/customerService/AISuggestion.vue |
| 智能体 | /chatChannel/list | 聊天渠道 | views/chatChannel/List.vue |
| 智能体 | /chatChannel/create | 新建渠道 | views/chatChannel/Create.vue |
| 智能体 | /chatChannel/edit/:id | 编辑渠道 | views/chatChannel/Edit.vue |
| 智能体 | /chatChannel/install-guide/:id? | 安装指引 | views/chatChannel/InstallGuide.vue |
| 智能体 | /objection/list | 异议处理 | views/objection/List.vue |
| 智能体 | /persona/list | 用户画像 | views/persona/List.vue |
| 触达运营 | /email | 邮件列表 | views/email/EmailList.vue |
| 触达运营 | /email/drafts | 邮件草稿箱 | views/email/Drafts.vue |
| 触达运营 | /email/jobs | 邮件发送任务 | views/email/Jobs.vue |
| 触达运营 | /email/smtp | SMTP 代理配置 | views/email/Smtp.vue |
| 触达运营 | /email/info | 邮箱服务商信息 | views/email/Info.vue |
| 触达运营 | /email/guide | 邮件营销指南 | views/email/Guide.vue |
| 触达运营 | /sms/list | 短信列表 | views/sms/List.vue |
| 触达运营 | /sms/drafts | 短信草稿 | views/sms/Drafts.vue |
| 触达运营 | /sms/jobs | 短信任务 | views/sms/Jobs.vue |
| 触达运营 | /sms/config | 短信配置 | views/sms/Config.vue |
| 触达运营 | /douyinCard | 抖音卡片管理 | views/douyinCard/List.vue |
| 触达运营 | /douyin/auto-reply | 抖音自动回复 | views/douyinCard/AutoReply.vue |
| 触达运营 | /douyin/stats | 抖音卡片统计 | views/douyinCard/Stats.vue |
| 触达运营 | /kuaishouCard | 快手卡片管理 | views/kuaishouCard/List.vue |
| 触达运营 | /kuaishou/auto-reply | 快手自动回复 | views/kuaishouCard/AutoReply.vue |
| 触达运营 | /kuaishou/stats | 快手卡片统计 | views/kuaishouCard/Stats.vue |
| 触达运营 | /xiaohongshuCard | 小红书卡片 | views/xiaohongshuCard/List.vue |
| 触达运营 | /xiaohongshu/auto-reply | 小红书自动回复 | views/xiaohongshuCard/AutoReply.vue |
| 触达运营 | /xiaohongshu/stats | 小红书卡片统计 | views/xiaohongshuCard/Stats.vue |
| 触达运营 | /xianyuCard | 闲鱼卡片管理 | views/xianyuCard/List.vue |
| 触达运营 | /xianyu/auto-reply | 闲鱼自动回复 | views/xianyuCard/AutoReply.vue |
| 触达运营 | /xianyu/stats | 闲鱼卡片统计 | views/xianyuCard/Stats.vue |
| 触达运营 | /tiktok/list | TikTok 卡片 | views/tiktokCard/List.vue |
| 触达运营 | /tiktok/stats | TikTok 统计 | views/tiktokCard/Stats.vue |
| 触达运营 | /tiktok/auto-reply | TikTok 自动回复 | views/tiktokCard/AutoReply.vue |
| 触达运营 | /whatsapp/account | WhatsApp 账号 | views/whatsapp/WhatsappAccount.vue |
| 触达运营 | /whatsapp/drafts | WhatsApp 草稿 | views/whatsapp/WhatsappDrafts.vue |
| 触达运营 | /whatsapp/jobs | WhatsApp 群发 | views/whatsapp/WhatsappJobs.vue |
| 触达运营 | /telegram/account | Telegram 机器人 | views/telegram/account.vue |
| 触达运营 | /feishu/account | 飞书账号 | views/feishu/FeishuAccount.vue |
| 触达运营 | /community/list | 社群管理 | views/community/List.vue |
| 触达运营 | /shortLink | 短链管理 | views/shortLink/List.vue |
| 触达运营 | /shortLink/stats | 短链统计 | views/shortLink/Stats.vue |
| 触达运营 | /livecode | 活码管理 | views/livecode/LiveCodeManagement.vue |
| 知识中心 | /knowledge/management | 知识库管理 | views/KnowledgeWorkspace/KnowledgeManagement.vue |
| 知识中心 | /knowledge/batch-import | 批量导入 | views/KnowledgeWorkspace/BatchImport.vue |
| 知识中心 | /knowledge/playground | 调试台 | views/KnowledgeWorkspace/Playground.vue |
| 知识中心 | /knowledge/chunks | 文本块(分段) | views/KnowledgeWorkspace/ChunkManagement.vue |
| 知识中心 | /knowledge/feedbacks | 反馈 | views/KnowledgeWorkspace/FeedbackList.vue |
| 知识中心 | /knowledge/tokens | API Token | views/KnowledgeWorkspace/ApiToken.vue |
| 知识中心 | /knowledge/external | 外部文档接入 | views/KnowledgeWorkspace/ExternalImport.vue |
| 知识中心 | /knowledge/statistics | 知识库统计 | views/KnowledgeWorkspace/KnowledgeStatistics.vue |
| 知识中心 | /knowledge/openapi | OpenAPI 数据源 | views/KnowledgeWorkspace/OpenAPIIntegration.vue |
| 知识中心 | /system/rag-product-config | RAG 产品配置 | views/RagProductConfig/index.vue |
| 知识中心 | /system/rag-account | RAG 账号配置 | views/RagProductConfig/AccountConfig.vue |
| 知识中心 | /system/rag-product | RAG 产品管理 | views/RagProductConfig/RagProductManagement.vue |
| 知识中心 | /system/rag-overview | RAG 概览 | views/system/RagOverview.vue |
| 知识中心 | /aiContent/list | AI 内容 | views/aiContent/List.vue |
| 知识中心 | /templateMarket/list | 模板市场 | views/templateMarket/List.vue |
| 数据分析 | /dashboardScreen/list | 营销数据大屏 | views/dashboardScreen/List.vue |
| 数据分析 | /conversionFunnel/list | 转化漏斗 | views/conversionFunnel/List.vue |
| 数据分析 | /aiProductivity/list | AI 生产力 | views/aiProductivity/List.vue |
| 数据分析 | /customReport/list | 自定义报表 | views/customReport/List.vue |
| 数据分析 | /abExperiment/list | A/B 实验 | views/abExperiment/List.vue |
| 数据分析 | /churnPrediction/list | 流失预测 | views/churnPrediction/List.vue |
| 数据分析 | /customerJourney/dashboard | 客户旅程 | views/customerJourney/Dashboard.vue |
| 系统设置 | /system/config | 系统基础配置 | views/system/Config.vue |
| 系统设置 | /system/obs-config | 对象存储配置 | views/system/ObsConfig.vue |
| 系统设置 | /system/material-library | 素材库 | views/system/MaterialLibrary.vue |
| 系统设置 | /system/monitor | 系统监控 | views/system/Monitor.vue |
| 系统设置 | /system/guide | 使用指南 | views/system/Guide.vue |
| 系统设置 | /domainPool | 域名池 | views/domainPool/List.vue |
| 系统设置 | /teamUser/list | 团队成员 | views/teamUser/List.vue |
| 系统设置 | /teamUser/role | 角色权限 | views/teamUser/Role.vue |
| 系统设置 | /platformAccount/list | 平台账号 | views/platformAccount/List.vue |
| 系统设置 | /payment/list | 支付记录 | views/payment/List.vue |
| 系统设置 | /payment/config | 支付配置 | views/payment/Config.vue |
| 系统设置 | /integration/list | 集成管理 | views/integration/List.vue |
| 系统设置 | /operationLog/list | 操作日志 | views/operationLog/List.vue |
| 系统设置 | /backup/list | 备份管理 | views/backup/List.vue |
| 系统设置 | /securityAudit/list | 安全审计 | views/securityAudit/List.vue |
| 独立 | /setup | 系统初始化向导 | views/setup/InitSetup.vue |
| 独立 | /login | 登录页 | views/Login.vue |
| 独立 | /profile | 个人中心 | views/Profile.vue |
| 独立 | /notifications | 通知中心 | views/Notifications.vue |
| 独立 | /chat/embed | 嵌入客服窗 | views/chat/embed/Index.vue |
| 独立 | NotFound | 404 | views/NotFound.vue |
| 独立 | /change-password | 修改密码 | views/ChangePassword.vue |
| 独立 | /forgot-password | 找回密码 | views/ForgotPassword.vue |

---

# 一、工作台（workspace）

## /messageHub/list — 消息中台 MQ
- **组件**：`views/messageHub/List.vue`
- **布局**：顶部标题+操作按钮，统计卡片行，平台分布条形图，搜索表单，消息表格，分页；含推送与详情弹窗。
- **UI 元素**：
  - 顶部操作：推送消息（主按钮）、刷新统计
  - 统计卡片：消息总数、接收消息、发送消息、未读消息、近24h新增、活跃平台数
  - 平台分布：各平台名称+条形+数量（有数据才显示）
  - 筛选区：平台（下拉）、账号ID（输入）、会话ID（输入）、发送者（输入）、方向（下拉）、类型（下拉）、已读（下拉）、群消息（下拉）、关键字（输入）、搜索、重置
  - 表格列：多选、ID、平台（标签）、账号、方向（标签）、类型（标签）、发送方、接收方、内容、会话（+群标签）、AI（标签）、状态（标签）、发送时间、操作（详情 / 标记已读-仅未读）
  - 批量条：已选 N 条 + 批量标记已读
  - 分页：total/sizes/prev/pager/next/jumper
  - 弹窗-推送：平台*、账号ID*、消息ID、方向*（单选）、消息类型*、发送方ID、发送方名称、接收方ID、接收方名称、会话ID、内容*（文本域）、媒体URL、群消息（开关+群ID）、AI回复（开关+Agent名）；按钮 取消/推送
  - 弹窗-详情：描述列表（ID、消息ID、平台、账号、方向、类型、发送方、接收方、会话ID、群消息、AI回复、已读、发送/读取/入库时间）+ 内容 + 媒体URL + 扩展字段 JSON
- **关键交互**：搜索/分页/标记已读刷新统计；推送带幂等去重校验。

## /wecomAccount/list — 企微账号管理
- **组件**：`views/wecomAccount/List.vue`
- **布局**：顶部 4 张健康度概览卡片，下方企微账号列表表格，绑定 AI 弹窗。
- **UI 元素**：
  - 概览卡片：账号总数（在线/离线）、平均健康分（满分100）、配额使用（日配额）、风险账号（需关注）
  - 列表头部：标题"企微账号管理" + 刷新按钮
  - 表格列：企业ID、应用ID、登录状态（标签）、风险等级（暗色标签）、健康分、好友/群组、日配额、最后活跃、操作（详情 / 绑定AI）
  - 弹窗-绑定AI：`AgentBindingDialog`（传入 channel-type=wecom, accountId, label, enabled）
- **关键交互**：进入加载列表+概览；详情按钮提示"开发中"；绑定AI 打开对话框关联 Agent。

---

# 二、客户中心（customer）

## /clue/list — 线索列表
- **组件**：`views/clue/List.vue`
- **布局**：筛选卡片 + 线索表格 + 新增/编辑/详情弹窗。
- **UI 元素**：
  - 筛选区：关键词、来源、状态、渠道、时间范围、搜索、重置
  - 表格列：选择框、ID、姓名、手机号、来源（标签）、渠道、状态（标签）、意向等级、归属人、创建时间、操作（查看/编辑/转为客户/删除）
  - 弹窗-新增/编辑：姓名*、手机号*、来源、渠道、状态、意向等级、备注（文本域）、标签；按钮 取消/保存
  - 弹窗-详情：描述列表（姓名/手机/来源/渠道/状态/意向/备注/创建时间）+ 跟进记录表
- **关键交互**：搜索/重置分页；转为客户二次确认。

## /clue/statistics — 线索统计
- **组件**：`views/clue/Statistics.vue`
- **布局**：顶部统计卡 + 图表（来源分布饼图、趋势折线）+ 转化漏斗卡。
- **UI 元素**：
  - 筛选区：时间范围、渠道
  - 指标卡：线索总数、有效线索、转化率、人均成本
  - 图表：来源分布（饼）、趋势（折线）、渠道对比（柱状）
- **关键交互**：切换时间/渠道刷新图表。

## /customer360/list — 客户 360
- **组件**：`views/customer360/List.vue`
- **布局**：客户搜索 + 360 详情抽屉（基本信息/标签/事件/订单/会话 Tab）。
- **UI 元素**：
  - 筛选区：手机号/姓名搜索、客户列表
  - 表格列：头像、姓名、手机号、标签、来源、最近活跃、操作（查看360）
  - 抽屉 Tab：基本信息（字段列表）、标签（标签组）、事件时间线、订单列表、会话记录
- **关键交互**：点击查看打开右侧抽屉，多 Tab 懒加载。

## /customerEvent/list — 客户事件
- **组件**：`views/customerEvent/List.vue`
- **布局**：筛选卡 + 事件表格 + 事件详情抽屉。
- **UI 元素**：
  - 筛选区：客户、事件类型、时间范围、搜索/重置
  - 表格列：ID、客户、事件类型（标签）、来源渠道、触发时间、操作（详情）
  - 抽屉：事件详情描述列表 + 扩展属性 JSON
- **关键交互**：分页/筛选；点击查看详情。

## /tagSegmentation/list — 标签分群
- **组件**：`views/tagSegmentation/List.vue`
- **布局**：标签分组树 + 标签列表 + 新建标签弹窗。
- **UI 元素**：
  - 左树：标签分组（可折叠）
  - 表格列：标签名、分组、覆盖人数、类型（手动/自动）、状态、创建时间、操作（编辑/删除）
  - 弹窗-新建标签：标签名*、分组（下拉）、类型（单选 手动/自动规则）、规则编辑器（自动）、备注
- **关键交互**：点击分组筛选标签；自动标签基于规则条件。

## /userSegment/list — 用户分群
- **组件**：`views/userSegment/List.vue`
- **布局**：分群列表 + 新建/编辑分群弹窗（条件组合器）。
- **UI 元素**：
  - 筛选区：名称搜索、状态
  - 表格列：分群名称、人数、规则摘要、状态（标签）、更新时间、操作（编辑/计算/删除）
  - 弹窗-编辑：分群名*、条件组（字段/运算符/值 多行组合 + AND/OR）、备注
- **关键交互**：保存后触发人数计算。

## /unifiedMessage/list — 统一消息
- **组件**：`views/unifiedMessage/List.vue`
- **布局**：渠道 Tab + 消息聚合表格 + 会话详情抽屉。
- **UI 元素**：
  - 渠道 Tab：微信/企微/WhatsApp/Telegram…
  - 筛选区：关键字、时间、已读
  - 表格列：会话头像、联系人、最后消息、渠道（标签）、未读数、时间、操作（打开）
  - 抽屉：消息气泡列表 + 输入框回复
- **关键交互**：点击会话打开右侧聊天窗，可回复。

## /order/list — 订单管理
- **组件**：`views/order/List.vue`
- **布局**：筛选卡 + 订单表格 + 详情弹窗。
- **UI 元素**：
  - 筛选区：订单号、客户、状态（待付款/已付款/已发货/完成/退款）、时间范围、搜索/重置
  - 表格列：订单号、客户、金额、商品数、状态（标签）、支付渠道、下单时间、操作（详情/退款）
  - 弹窗-详情：商品明细表（名称/单价/数量/小计）+ 收货信息 + 金额汇总
- **关键交互**：退款需二次确认。

## /oneid/list — OneID 合并
- **组件**：`views/oneid/List.vue`
- **布局**：跨渠道身份列表 + 合并弹窗。
- **UI 元素**：
  - 筛选区：身份标识、渠道、是否合并
  - 表格列：主ID、关联渠道数、渠道标识列表、合并状态、操作（查看关联/合并）
  - 弹窗-合并：选择主记录 + 待合并记录 + 字段冲突解决
- **关键交互**：选择多条记录执行合并，冲突字段人工选择。

## /oneid/conflicts — OneID 冲突
- **组件**：`views/oneid/Conflicts.vue`
- **布局**：冲突记录表格 + 处理弹窗。
- **UI 元素**：
  - 筛选区：冲突类型、处理状态
  - 表格列：冲突ID、涉及记录、冲突字段、置信度、状态（待处理/已解决）、操作（处理/忽略）
  - 弹窗-处理：冲突说明 + 解决方案选择（保留A/保留B/合并）
- **关键交互**：处理或忽略冲突记录。

---

# 三、智能体（aiAgent）

## /aiAgent/list — 智能体列表
- **组件**：`views/aiAgent/List.vue`
- **布局**：头部卡片 + 筛选卡片 + 表格卡片三段式，标题"AI 智能体管理"。
- **UI 元素**：
  - 头部：刷新按钮、新建智能体（primary）
  - 筛选区：智能体类型（下拉 销售/客服/混合，可清）、状态（下拉 启用1/禁用0，可清）、关键词（输入 名称/编码，回车/清空触发）、搜索、重置
  - 表格列：ID、头像、名称、编码、类型（标签）、状态（标签）、LLM模型、人设摘要（截断50）、创建时间、操作（编辑 / 禁用or启用 / 测试 / 绑定关系 / 删除）
  - 测试弹窗：智能体（禁用）、客户ID（输入）、消息内容（必填文本域）+ 执行测试、清空结果；结果区：回复内容/LLM模型/耗时/消耗Token/是否转人工 + 9步链路日志表（序号/步骤/状态/耗时/详情/错误）
  - 绑定关系弹窗：智能体名称+编码描述、渠道账号绑定表（ID/渠道类型/账号ID/主绑定/启用/创建时间）
- **关键交互**：筛选即时查询；操作列确认框后执行启用禁用与删除，测试与绑定以弹窗展示。

## /aiAgent/create — 创建智能体
- **组件**：`views/aiAgent/Edit.vue`（isEdit=false，路由 /aiAgent/create；编辑 /aiAgent/edit/:id 复用同组件，编码禁用）
- **布局**：头部卡片 + 分区块表单卡片（基本信息/人设/知识库/SOP/话术/LLM/引擎开关/高级参数）+ 底部按钮。
- **UI 元素**：
  - 头部：返回列表、测试（primary）
  - 基本信息：智能体编码*（输入,正则校验）、名称*、智能体类型*（下拉 销售/客服/混合）、头像URL、描述（文本域）
  - 人设配置：人设描述*（文本域）、系统提示词（文本域）、欢迎语
  - 知识库挂载：RAG 产品（多选过滤下拉）
  - SOP 挂载：SOP 列表（多选过滤下拉）
  - 话术库挂载：话术模板（多选过滤下拉）
  - LLM 配置：LLM模型（下拉8项）、最大Token（数字 100~8192）、温度（滑块 0~2）、Top P（滑块 0~1）、频率惩罚（滑块 -2~2）、存在惩罚（滑块 -2~2）
  - 引擎开关：RAG检索 / 话术匹配 / 拟人化润色 / 内容审核 / 销冠话术（5 个 switch）
  - 高级参数：RAG TopK（数字 1~20）、置信度阈值（滑块 0~1）、AI连续回复上限（数字 1~50）
  - 底部：取消、创建智能体（primary,loading）
  - 测试弹窗：同列表（创建模式禁止测试，需先保存）
- **关键交互**：校验必填后 createAgent 保存跳回列表；未保存不可测试。

## /unifiedInbox/list — 统一收件箱
- **组件**：`views/unifiedInbox/List.vue`
- **布局**：渠道会话列表 + 消息区 + 回复框 + 智能体接管开关。
- **UI 元素**：
  - 会话列表：头像、联系人、最后消息、未读徽标、渠道标签
  - 消息区：气泡列表（时间/方向）
  - 工具栏：分配坐席、转 AI、标记已读、备注
  - 回复框：文本域 + 快捷话术插入 + 发送；AI 建议区（来自 /customerService/aiSuggestion）
- **关键交互**：点击会话加载历史；可人工回复或交由智能体。

## /customerSession/list — 客户会话
- **组件**：`views/customerSession/List.vue`
- **布局**：会话列表 + 详情抽屉（消息时间线 + 标签 + AI 摘要）。
- **UI 元素**：
  - 筛选区：渠道、坐席、时间、关键词
  - 表格列：会话ID、客户、渠道、坐席、状态（进行中/已结束）、开始/结束时间、操作（详情）
  - 抽屉：消息时间线、会话标签、AI 摘要、转人工标记
- **关键交互**：查看会话详情与摘要。

## /intentRecognition/list — 意图识别
- **组件**：`views/intentRecognition/List.vue`
- **布局**：意图列表 + 新建/编辑弹窗（意图词/例句/正则）。
- **UI 元素**：
  - 筛选区：名称搜索、状态
  - 表格列：意图名、类别、例句数、命中数、状态、操作（编辑/删除）
  - 弹窗-编辑：意图名*、类别*、训练例句（多行）、匹配正则、关联话术、备注
- **关键交互**：保存意图用于 NLU 路由。

## /dialogueMemory/list — 对话记忆
- **组件**：`views/dialogueMemory/List.vue`
- **布局**：记忆条目列表 + 查看弹窗。
- **UI 元素**：
  - 筛选区：客户、记忆类型、时间
  - 表格列：客户、记忆类型（偏好/事实/历史）、内容摘要、更新时间、操作（查看/删除）
  - 弹窗-查看：完整记忆内容 + 来源会话
- **关键交互**：查看或删除记忆。

## /llmRouting/list — LLM 路由
- **组件**：`views/llmRouting/List.vue`
- **布局**：路由规则列表 + 新增/编辑弹窗（条件 → 模型）。
- **UI 元素**：
  - 表格列：规则名、匹配条件（渠道/意图/负载）、目标模型、优先级、启用（开关）、操作（编辑/删除）
  - 弹窗-编辑：规则名*、条件组、目标 LLM 模型*、优先级、启用开关
- **关键交互**：规则按优先级路由请求到不同 LLM。

## /reachPipeline/list — 触达流水线
- **组件**：`views/reachPipeline/List.vue`
- **布局**：流水线画布（节点拖拽）+ 节点配置抽屉。
- **UI 元素**：
  - 工具栏：新建流程、保存、运行
  - 画布节点：触发/筛选/渠道/延时/分支 等节点
  - 节点配置：选中节点右侧抽屉编辑参数
- **关键交互**：拖拽编排触达流程并保存运行。

## /marketingFlow/list — 营销流程
- **组件**：`views/marketingFlow/List.vue`
- **布局**：流程列表 + 设计器弹窗（类似 reachPipeline）。
- **UI 元素**：
  - 表格列：流程名、触发事件、状态、最近运行、操作（设计/启用/删除）
  - 设计器：步骤节点（进入/条件/动作/结束）+ 配置面板
- **关键交互**：设计营销自动化流程。

## /batchOperation/list — 批量操作
- **组件**：`views/batchOperation/List.vue`
- **布局**：任务列表 + 新建批量任务弹窗（选人群 + 动作）。
- **UI 元素**：
  - 筛选区：任务类型、状态、时间
  - 表格列：任务名、目标分群、动作类型、进度、状态、操作（详情/停止）
  - 弹窗-新建：任务名*、目标分群*、动作（打标签/发消息/改字段）、参数、调度（立即/定时）
- **关键交互**：提交后异步执行，可查看进度。

## /sopAgent/list — SOP 智能体
- **组件**：`views/sopAgent/List.vue`
- **布局**：SOP 列表 + 编辑弹窗（步骤树 + 话术）。
- **UI 元素**：
  - 表格列：SOP名、适用场景、关联智能体、步骤数、状态、操作（编辑/删除）
  - 弹窗-编辑：SOP名*、场景、步骤编辑器（步骤名/触发条件/话术/分支）
- **关键交互**：编辑 SOP 步骤供智能体执行。

## /scriptTemplate/list — 话术模板
- **组件**：`views/scriptTemplate/List.vue`
- **布局**：模板列表 + 新建/编辑弹窗（分类 + 变量）。
- **UI 元素**：
  - 筛选区：分类、关键词
  - 表格列：模板名、分类、内容预览、使用次数、状态、操作（编辑/复制/删除）
  - 弹窗-编辑：模板名*、分类*、内容*（支持 {变量}）、备注
- **关键交互**：模板用于话术匹配与快捷回复。

## /customerService/agentStatus — 客服坐席状态
- **组件**：`views/customerService/AgentStatus.vue`
- **布局**：坐席卡片网格（在线状态/当前会话数/接待量）。
- **UI 元素**：
  - 筛选区：状态（在线/忙碌/离线）、搜索
  - 坐席卡：头像、姓名、状态标签、进行中会话数、今日接待、平均响应
  - 操作：强制下线、查看详情
- **关键交互**：监控坐席实时状态。

## /customerService/quickReply — 快捷回复
- **组件**：`views/customerService/QuickReply.vue`
- **布局**：分组 + 快捷语列表 + 编辑弹窗。
- **UI 元素**：
  - 左树：分组
  - 表格列：标题、内容、分组、热度、操作（编辑/删除）
  - 弹窗-编辑：标题*、分组、内容*、快捷键
- **关键交互**：坐席在会话中插入快捷语。

## /customerService/sessionTag — 会话标签
- **组件**：`views/customerService/SessionTag.vue`
- **布局**：标签管理列表 + 编辑弹窗。
- **UI 元素**：
  - 表格列：标签名、颜色、使用次数、操作（编辑/删除）
  - 弹窗-编辑：标签名*、颜色（取色器）、描述
- **关键交互**：用于标记会话主题。

## /customerService/aiSuggestion — AI 辅助建议
- **组件**：`views/customerService/AISuggestion.vue`
- **布局**：配置页（开关 + 触发条件）+ 建议示例。
- **UI 元素**：
  - 表单：启用 AI 建议（开关）、触发时机、模型选择、话术来源、最大建议条数
  - 示例区：模拟会话展示 AI 推荐话术卡片
- **关键交互**：配置后坐席回复时展示 AI 推荐。

## /chatChannel/list — 聊天渠道
- **组件**：`views/chatChannel/List.vue`
- **布局**：渠道列表 + 新建/编辑/安装指引入口。
- **UI 元素**：
  - 表格列：渠道名称、类型（网页/微信/WhatsApp…）、状态、绑定智能体、创建时间、操作（编辑/安装指引/删除）
  - 弹窗-新建：渠道名*、类型*、绑定智能体、站点域名、外观配置
- **关键交互**：新建/编辑渠道；查看安装代码片段。

## /chatChannel/create — 新建渠道
- **组件**：`views/chatChannel/Create.vue`
- **布局**：分步表单（基础信息 → 外观 → 智能体绑定）。
- **UI 元素**：
  - 表单：渠道名*、类型*、站点域名*、主题色、欢迎语、绑定智能体、启用开关
  - 按钮：取消/创建
- **关键交互**：创建后生成嵌入脚本。

## /chatChannel/edit/:id — 编辑渠道
- **组件**：`views/chatChannel/Edit.vue`
- **布局**：同新建，字段预填。
- **UI 元素**：同 Create，编辑时回填并支持保存。

## /chatChannel/install-guide/:id? — 安装指引
- **组件**：`views/chatChannel/InstallGuide.vue`
- **布局**：步骤卡片 + 代码片段（可复制）。
- **UI 元素**：
  - 步骤：引入脚本 / 放置挂载点 / 配置域名
  - 代码块：`<script src=...>` 嵌入片段 + 复制按钮
- **关键交互**：复制代码嵌入网站。

## /objection/list — 异议处理
- **组件**：`views/objection/List.vue`
- **布局**：异议库列表 + 编辑弹窗（异议 → 应对话术）。
- **UI 元素**：
  - 表格列：异议类型、客户说法、应对话术、命中数、操作（编辑/删除）
  - 弹窗-编辑：类型*、常见表述*、标准应对*、关联意图
- **关键交互**：用于销售场景智能体应对。

## /persona/list — 用户画像
- **组件**：`views/persona/List.vue`
- **布局**：画像列表 + 编辑弹窗（属性 + 特征）。
- **UI 元素**：
  - 表格列：画像名、人群规模、关键特征、状态、操作（编辑/删除）
  - 弹窗-编辑：画像名*、描述、特征标签、关联分群
- **关键交互**：定义目标人群画像。

---

# 四、触达运营（reach）

## /email — 邮件列表
- **组件**：`views/email/EmailList.vue`
- **布局**：单卡片内表格展示邮件发送记录，带分页。
- **UI 元素**：
  - 筛选区：无（仅标题"邮件列表"）
  - 表格列：主题、收件人、发件人、是否发送（标签）、发送时间、是否阅读（标签）、阅读时间
- **关键交互**：分页切换重新拉取邮件列表。

## /email/drafts — 邮件草稿箱
- **组件**：`views/email/Drafts.vue`
- **布局**：标题+新建按钮，草稿表格，新建/编辑弹窗（富文本）。
- **UI 元素**：
  - 表格列：选择框、主题、操作（编辑/删除/发送）
  - 弹窗-表单：主题*（输入）、内容*（SimpleEditor 富文本）；按钮 取消/保存草稿
  - 变量提示：{name}{city}{address}{account}
- **关键交互**：行双击或编辑进弹窗；发送直接调接口提示写入条数。

## /email/jobs — 邮件发送任务
- **组件**：`views/email/Jobs.vue`
- **布局**：任务统计表格 + 分页。
- **UI 元素**：
  - 表格列：主题、计划条数、已发送、成功、失败、已读、未读（计算）、创建时间
- **关键交互**：分页切换拉取任务列表。

## /email/smtp — SMTP 邮件代理配置
- **组件**：`views/email/Smtp.vue`
- **布局**：工具栏+代理表格+新增/编辑弹窗。
- **UI 元素**：
  - 表格列：代理名称、服务器地址、端口、代理日限制、密码、用户名、操作（编辑/删除）
  - 弹窗-表单：代理名称*、服务器地址*、端口*（数字）、用户名*、密码*、代理日限制*；按钮 取消/确定
- **关键交互**：删除需确认；保存后刷新列表。

## /email/info — 邮箱服务商信息
- **组件**：`views/email/Info.vue`
- **布局**：只读表格展示主流邮箱 SMTP 配置参考。
- **UI 元素**：
  - 筛选区：searchText（计算过滤）
  - 表格列：名称、厂家、SMTP服务器（标签）、端口（标签 ssl/tls/plain）、认证要求（图标+悬浮）、注册地址（链接）、操作（前往注册）
- **关键交互**：点击"前往注册"新开注册页。

## /email/guide — 邮件营销使用指南
- **组件**：`views/email/Guide.vue`
- **布局**：步骤条 + 四张步骤卡片 + 常见问题。
- **UI 元素**：
  - 卡片列表：配置SMTP / 收件人列表 / 编写草稿 / 发送任务（含详情与"前往操作"链接）
  - FAQ：SMTP 常见问题
- **关键交互**：点击卡片跳对应路由。

## /sms/list — 短信列表
- **组件**：`views/sms/List.vue`
- **布局**：卡片内筛选表单 + 短信表格 + 分页。
- **UI 元素**：
  - 筛选区：手机号（输入）、状态（选择 待发送/发送中/已发送/失败）、发送时间（日期范围）、搜索/重置
  - 表格列：ID、手机号、短信内容、状态（标签）、发送时间、操作（查看 / 重发[失败]）
- **关键交互**：搜索/重置分页拉取；失败可重发（查看用 alert 弹 JSON）。

## /sms/drafts — 短信草稿
- **组件**：`views/sms/Drafts.vue`
- **布局**：卡片+草稿表格+两个弹窗（草稿/发送）。
- **UI 元素**：
  - 表格列：ID、标题、内容、创建时间、更新时间、操作（编辑/发送/删除）
  - 弹窗-草稿：标题*（限100）、内容*（文本域限500）
  - 弹窗-发送：手机号*（限11）、内容预览；按钮 取消/确定
- **关键交互**：发送弹窗校验11位手机号后提交。

## /sms/jobs — 短信任务
- **组件**：`views/sms/Jobs.vue`
- **布局**：卡片+任务表格+创建弹窗+详情弹窗。
- **UI 元素**：
  - 表格列：ID、任务名称、总数、已发送、失败数、状态（标签）、计划发送时间、创建时间、操作（查看/暂停[running]/继续[paused]/停止[running,paused]/删除[completed,failed]）
  - 弹窗-创建：任务名称*、选择草稿*、内容预览、接收人群*（手机号输入+添加+标签列表+批量上传 csv/txt+从线索库）、发送方式（立即/定时）、定时时间*（日期）
  - 弹窗-详情：描述列表（名称/总数/状态等）+ 发送记录表（手机号、状态、发送时间、错误信息）
- **关键交互**：创建校验手机号与定时；详情查看发送记录。

## /sms/config — 短信配置
- **组件**：`views/sms/Config.vue`
- **布局**：单卡片表单，分三家云商配置段。
- **UI 元素**：
  - 表单：默认短信平台（选择 阿里/腾讯/华为）、发送频率限制（数字 1-1000 条/分）、每日发送限制（100-100000 条/天）、失败重试次数（0-5）
  - 阿里云：AccessKey ID、AccessKey Secret（密码）、短信签名
  - 腾讯云：SecretId、SecretKey（密码）、短信应用ID、短信签名
  - 华为云：APP Key、APP Secret（密码）、通道号、短信签名
  - 按钮：保存配置
- **关键交互**：保存提交全量配置。

## /douyinCard — 抖音卡片管理
- **组件**：`views/douyinCard/List.vue`
- **布局**：卡片+搜索+表格+添加/编辑弹窗（左表单右预览）+ 详情弹窗。
- **UI 元素**：
  - 筛选区：关键词（标题/描述/标签）、状态（激活/禁用）、搜索/重置
  - 表格列：ID、标题、描述、图片（缩略）、浏览数、状态（标签）、创建时间、操作（编辑/删除/统计/复制链接）
  - 弹窗-添加/编辑：标题*、描述、图片URL*（URL）、跳转链接*（URL）、域名池（选择）、标签、状态（开关）；按钮 取消/确定
  - 弹窗-详情：描述列表（ID/标题/描述/平台/状态/跳转链接/短链接/标签/浏览数/图片）；按钮 关闭
- **关键交互**：编辑预填；复制链接用短链或生成；统计跳详情页。

## /douyin/auto-reply — 抖音自动回复
- **组件**：`views/douyinCard/AutoReply.vue`
- **布局**：上双栏（账号表+规则表单）下日志表 + 登录弹窗。
- **UI 元素**：
  - 筛选区：账号输入（昵称）+ 绑定账号
  - 账号表：账号、状态、操作（删除）
  - 日志表：时间、目标、回复、状态
  - 规则表单：关键词（文本域）、话术（文本域）、频率秒（数字）、每日上限（数字）、启用（开关）、启用RAG（开关）、选择RAG产品（选择）；按钮 保存规则/启动/停止
  - 登录弹窗：平台/昵称展示、二维码/手动 Cookie 文本域/无头模式开关；按钮 取消/保存账号
- **关键交互**：绑定触发浏览器扫码；轮询登录状态后保存 Cookie。

## /douyin/stats — 抖音卡片总统计
- **组件**：`views/douyinCard/Stats.vue`
- **布局**：卡片+三统计卡+趋势图+热门/最近活动表。
- **UI 元素**：
  - 筛选区：日期范围（默认近7天）、分组方式（天/周/月）、刷新
  - 统计卡：总浏览量、访客数、转化数、分享数
  - 热门卡片表：标题、浏览量、操作（统计）
  - 最近活动表：卡片标题、操作（标签）、用户、时间
  - 图表：ECharts 访问趋势图
- **关键交互**：改日期/分组刷新图表；热门统计跳单卡页。

## /douyin-card-stats/:id — 单个抖音卡片统计
- **组件**：`views/douyinCard/CardStats.vue`
- **布局**：卡片详情+总浏览量卡+趋势图+最近活动表。
- **UI 元素**：
  - 筛选区：返回、日期范围、分组方式（天/周/月）、刷新
  - 卡片信息：图片、ID、标题、状态（标签）、创建时间、描述
  - 最近活动表：用户、操作（标签）、IP地址、时间
  - 图表：ECharts 访问趋势图
- **关键交互**：改日期/分组刷新；返回上一页。

## /kuaishouCard — 快手卡片管理
- **组件**：`views/kuaishouCard/List.vue`
- **布局**：同抖音卡片（关键词/状态筛选 + 表格 + 编辑弹窗 + 详情弹窗 + 统计/复制链接）。
- **UI 元素**：
  - 筛选区：关键词、状态（激活/禁用）、搜索/重置
  - 表格列：ID、标题、描述、图片、浏览数、状态、创建时间、操作（编辑/删除/统计/复制链接）
  - 弹窗-编辑：标题*、描述、图片URL*、跳转链接*、域名池、标签、状态（开关）
  - 弹窗-详情：描述列表（ID/标题/描述/状态/跳转链接/短链接/标签/浏览数/图片）
- **关键交互**：同抖音卡片管理。

## /kuaishou/auto-reply — 快手自动回复
- **组件**：`views/kuaishouCard/AutoReply.vue`
- **布局**：同抖音自动回复（账号表+规则表单+日志表+登录弹窗，平台为快手）。
- **UI 元素**：账号表/日志表/规则表单（关键词、话术、频率秒、每日上限、启用、启用RAG、选择RAG产品）/ 登录弹窗（二维码/手动Cookie/无头模式）。
- **关键交互**：绑定账号后启停自动回复。

## /kuaishou/stats — 快手卡片统计
- **组件**：`views/kuaishouCard/Stats.vue`
- **布局**：同抖音卡片总统计（三统计卡 + 趋势图 + 热门/最近活动表）。
- **UI 元素**：日期范围、分组方式、统计卡、热门卡片表、最近活动表、ECharts 趋势图。
- **关键交互**：改日期/分组刷新图表。

## /xiaohongshuCard — 小红书卡片
- **组件**：`views/xiaohongshuCard/List.vue`
- **布局**：同抖音卡片（小红书平台）。
- **UI 元素**：筛选区（关键词/状态）、表格列（ID/标题/描述/图片/浏览数/状态/创建时间/操作）、编辑弹窗（标题*/描述/图片URL*/跳转链接*/域名池/标签/状态）、详情弹窗。
- **关键交互**：编辑/统计/复制链接。

## /xiaohongshu/auto-reply — 小红书自动回复
- **组件**：`views/xiaohongshuCard/AutoReply.vue`
- **布局**：同抖音自动回复（平台为小红书）。
- **UI 元素**：账号表/日志表/规则表单/登录弹窗。
- **关键交互**：绑定账号后启停。

## /xiaohongshu/stats — 小红书卡片统计
- **组件**：`views/xiaohongshuCard/Stats.vue`
- **布局**：同抖音卡片总统计。
- **UI 元素**：统计卡 + 趋势图 + 热门/最近活动。
- **关键交互**：改日期/分组刷新。

## /xianyuCard — 闲鱼卡片管理
- **组件**：`views/xianyuCard/List.vue`
- **布局**：同抖音卡片（闲鱼平台）。
- **UI 元素**：筛选区（关键词/状态）、表格列（ID/标题/描述/图片/浏览数/状态/创建时间/操作）、编辑弹窗、详情弹窗。
- **关键交互**：编辑/统计/复制链接。

## /xianyu/auto-reply — 闲鱼自动回复
- **组件**：`views/xianyuCard/AutoReply.vue`
- **布局**：同抖音自动回复（平台为闲鱼）。
- **UI 元素**：账号表/日志表/规则表单/登录弹窗。
- **关键交互**：绑定账号后启停。

## /xianyu/stats — 闲鱼卡片统计
- **组件**：`views/xianyuCard/Stats.vue`
- **布局**：同抖音卡片总统计。
- **UI 元素**：统计卡 + 趋势图 + 热门/最近活动。
- **关键交互**：改日期/分组刷新。

## /tiktok/list — TikTok 卡片
- **组件**：`views/tiktokCard/List.vue`
- **布局**：同抖音卡片（TikTok 平台）。
- **UI 元素**：筛选区（关键词/状态）、表格列（ID/标题/描述/图片/浏览数/状态/创建时间/操作）、编辑弹窗、详情弹窗。
- **关键交互**：编辑/统计/复制链接。

## /tiktok/stats — TikTok 统计
- **组件**：`views/tiktokCard/Stats.vue`
- **布局**：同抖音卡片总统计。
- **UI 元素**：统计卡 + 趋势图 + 热门/最近活动。
- **关键交互**：改日期/分组刷新。

## /tiktok/auto-reply — TikTok 自动回复
- **组件**：`views/tiktokCard/AutoReply.vue`
- **布局**：同抖音自动回复（平台为 TikTok）。
- **UI 元素**：账号表/日志表/规则表单/登录弹窗。
- **关键交互**：绑定账号后启停。

## /whatsapp/account — WhatsApp 账号管理
- **组件**：`views/whatsapp/WhatsappAccount.vue`
- **布局**：el-card 内含行内表单与账号表格，底部配扫码登录弹窗。
- **UI 元素**：
  - 筛选区：名称（el-input）、备注（el-input）、添加账号（primary）
  - 表格列：名称、备注、状态、操作（登录 / 状态 / 绑定AI）
  - 弹窗-扫码登录：二维码（img 300×300, qrserver 生成）
  - 弹窗-绑定AI：复用 `AgentBindingDialog`（channel-type=whatsapp）
- **关键交互**：点击登录/状态获取二维码弹窗；绑定AI 打开智能体绑定对话框。

## /whatsapp/drafts — WhatsApp 草稿箱
- **组件**：`views/whatsapp/WhatsappDrafts.vue`
- **布局**：卡片头含"新建草稿"按钮，行内搜索表单 + 带分页表格 + 编辑/新建弹窗。
- **UI 元素**：
  - 筛选区：标题（el-input,回车搜索）、搜索（primary）
  - 表格列：ID、标题、内容（悬浮提示）、更新时间、操作（编辑/删除）
  - 弹窗-新建/编辑：标题*（el-input）、内容*（el-textarea）、取消/保存
- **关键交互**：删除走 ElMessageBox 确认；分页 10/20/50 触发 reload。

## /whatsapp/jobs — WhatsApp 群发任务
- **组件**：`views/whatsapp/WhatsappJobs.vue`
- **布局**：卡片头"新建任务"按钮 + 状态筛选 + 表格 + 两个弹窗。
- **UI 元素**：
  - 筛选区：状态（el-select 待执行/执行中/已完成/已失败）、搜索
  - 表格列：ID、草稿、状态（el-tag）、总数、已发、失败、创建时间、操作（详情/删除）
  - 弹窗-新建任务：选择草稿*（el-select）、目标号码*（el-textarea 每行/逗号分隔）、取消/创建
  - 弹窗-详情：el-descriptions（ID/草稿/状态/总数/已发/失败/创建时间/完成时间）
- **关键交互**：创建按行/逗号拆分号码；详情调 getJob 回填描述。

## /telegram/account — Telegram 机器人管理
- **组件**：`views/telegram/account.vue`
- **布局**：搜索框+添加/刷新按钮，宽表格（操作列5按钮），含编辑/测试/绑定AI 三弹窗。
- **UI 元素**：
  - 筛选区：账号名称（el-input 本地过滤）、添加机器人（primary）、刷新（info）
  - 表格列：账号名称、Bot Token（掩码）、Webhook URL、Webhook（el-tag 已/未注册）、智能体（el-tag 已/未启用）、状态（正常/停用）、最近错误、操作（编辑/注册Webhook/测试发送/绑定AI/删除）
  - 弹窗-添加/编辑：账号名称*、Bot Token*（password,编辑可留空）、Webhook URL、Webhook Secret、启用Webhook（switch）、启用智能体（switch）、状态（radio 正常/停用）、取消/确认
  - 弹窗-测试发送：目标 Chat ID*（number）、消息内容*（textarea）、取消/发送
  - 弹窗-绑定AI：复用 `AgentBindingDialog`（channel-type=telegram）
- **关键交互**：注册 Webhook 二次确认；删除走 ElMessageBox。

## /feishu/account — 飞书账号管理
- **组件**：`views/feishu/FeishuAccount.vue`
- **布局**：同 TG 结构，表格含 Token 缓存列，含编辑/测试/绑定AI 三弹窗。
- **UI 元素**：
  - 筛选区：账号名称（el-input 本地过滤）、添加飞书账号（primary）、刷新（info）
  - 表格列：账号名称、App ID、状态（正常/停用）、Webhook（el-tag）、智能体（el-tag）、Token（el-tooltip 已/未缓存）、最近错误、操作（编辑/测试发送/刷新Token/绑定AI/删除）
  - 弹窗-添加/编辑：账号名称*、App ID*、App Secret*（password,编辑留空）、Verification Token、Encrypt Key（password）、启用Webhook（switch）、启用智能体（switch）、状态（radio 正常/停用）、取消/确定
  - 弹窗-测试发送：接收者*（open_id/chat_id）、消息内容*（textarea）、消息类型（el-select text/post/interactive）、取消/发送
  - 弹窗-绑定智能体：账号（disabled）、智能体（switch）、Webhook（switch）、取消/保存
- **关键交互**：刷新 Token 直接调用；绑定AI 走独立对话框调 updateAccount。

## /community/list — 社群管理
- **组件**：`views/community/List.vue`
- **布局**：标题+导出/导入/新增按钮，分组搜索表单+主表格，4 个明细弹窗。
- **UI 元素**：
  - 筛选区：分组名称（el-input）、状态（el-select 正常/禁用）、搜索、重置
  - 表格列：ID、分组名称、描述、成员数、消息数、状态（el-tag）、创建时间、操作（编辑/成员/消息/统计/删除）
  - 弹窗-新增/编辑分组：分组名称*（2-50字）、描述（textarea）、状态（radio 正常/禁用）、取消/确定
  - 弹窗-社群成员：表（ID、昵称、角色、加入时间）+ 分页
  - 弹窗-社群消息：表（ID、发送者、内容、类型、时间）+ 分页
  - 弹窗-社群统计：3 卡片（成员/消息总数、今日活跃）+ 描述（名称、ID、创建时间、状态）
- **关键交互**：导出/导入调接口；成员、消息、统计各自分页独立加载。

## /shortLink — 短链管理
- **组件**：`views/shortLink/List.vue`
- **布局**：标题+添加按钮，搜索表单+表格，含编辑/统计/分享三弹窗。
- **UI 元素**：
  - 筛选区：短码（el-input）、原始URL（el-input）、状态（el-select 正常/禁用）、搜索、重置
  - 表格列：ID、短码、原始URL、标题、描述、点击次数、状态（el-tag 正常/禁用/过期）、过期时间（空显永不过期）、创建时间、操作（编辑/统计/分享/删除）
  - 弹窗-添加/编辑：短码*（4-20字母数字,带"生成"按钮）、原始URL*（url）、标题、描述（textarea）、访问密码（password）、过期时间（datetime）、状态（编辑时显示 select）、取消/确定
  - 弹窗-短链统计：4 卡片（累计/今日/昨日/日均访问）+ 七日趋势折线图 + 设备饼图（ECharts）
  - 弹窗-分享：短链地址（readonly+复制）、二维码（img）
- **关键交互**：生成短码随机6位；统计/分享弹窗独立加载并渲染图表。

## /shortLink/stats — 短链统计
- **组件**：`views/shortLink/Stats.vue`
- **布局**：标题+总体统计按钮，搜索（无状态）+ 表格，三弹窗（单链统计/总体统计/分享）。
- **UI 元素**：
  - 筛选区：短码（el-input）、原始URL（el-input）、搜索、重置
  - 表格列：ID、短码、原始URL、标题、累计点击次数、状态（el-tag）、操作（统计/分享）
  - 弹窗-单链统计：4 卡片（累计/今日/昨日/日均访问）+ 七日趋势折线 + 设备饼图
  - 弹窗-总体统计：4 卡片（累计访问、今日访问、短链总数、活跃短链数）+ 趋势 + 饼图
  - 弹窗-分享：短链地址（readonly+复制）、二维码（img）
- **关键交互**：总体统计调 getAllStats 渲染全站图表；其余与 /shortLink 分享/单链统计一致。

## /livecode — 活码管理
- **组件**：`views/livecode/LiveCodeManagement.vue`
- **布局**：筛选卡 + 活码列表表格 + 新建/编辑弹窗（含二维码预览）。
- **UI 元素**：
  - 筛选区：名称/关键词、类型（个人/群活码）、状态、搜索、重置
  - 表格列：活码名称、类型（标签）、二维码（缩略）、扫描次数、状态（标签）、创建时间、操作（编辑/下载二维码/删除）
  - 弹窗-新建/编辑：活码名称*、类型*（个人/群）、关联账号/群、备注、有效期（可选）、状态（开关）；按钮 取消/保存
  - 二维码预览：当前配置生成的活码图片
- **关键交互**：保存后生成活码；下载二维码图片。

---

# 五、知识中心（knowledge）

## /knowledge/management — 知识库管理
- **组件**：`views/KnowledgeWorkspace/KnowledgeManagement.vue`
- **布局**：顶部 6 张统计卡 + 工具栏 + 文档表格 + 导入/详情弹窗。
- **UI 元素**：
  - 统计卡：文档总数、分段总数、总Token、今日导入、索引就绪率、检索命中率
  - 筛选区：产品下拉、嵌入状态下拉（待处理/处理中/已索引/失败）、来源类型下拉（文件/文本/URL/OpenAPI）、标题搜索框、搜索按钮
  - 表格列：ID、标题（链接）、来源（标签）、分类、大小、分段数、状态（标签+进度条）、导入时间、操作（详情/重建索引/删除）
  - 导入弹窗（标签页）：文件上传（产品*、标题、分类、文件*）/ 文本粘贴（产品*、标题、内容*）/ URL抓取（产品*、URL*、标题）
  - 详情弹窗：描述列表（文档ID/标题/来源/分类/文件类型/大小/分段数/Tokens/检索次数/命中次数/状态/导入时间/最后索引/错误）+ 分段预览表（序号、内容、Tokens、字符数）+ 打开分段编辑器按钮
  - 工具栏：导入资料、刷新
- **关键交互**：筛选即时搜索，导入后轮询进度，标题/详情可查看分段。

## /knowledge/batch-import — 批量导入
- **组件**：`views/KnowledgeWorkspace/BatchImport.vue`
- **布局**：顶部提示 + 左配置右预览双栏 + 结果卡 + 帮助卡。
- **UI 元素**：
  - 提示：el-alert 说明两种方式
  - 左栏标签页：文件上传（产品*、格式单选 auto/CSV/JSON、文件*）+ 预览按钮；JSON粘贴（产品*、JSON内容*、解析预览/载入模板按钮）
  - 右栏预览表：标题、分类、内容预览、标签 + 确认导入按钮（显示条数）
  - 结果卡：总数/成功/失败/成功率 统计 + 失败明细表（序号、错误信息）+ 关闭
  - 帮助卡：CSV 格式说明与表头示例
- **关键交互**：本地解析预览，确认导入弹确认框，展示批号与失败明细。

## /knowledge/playground — 调试台
- **组件**：`views/KnowledgeWorkspace/Playground.vue`
- **布局**：顶部提示 + 左参数右结果双栏。
- **UI 元素**：
  - 左栏检索参数表单：产品*、查询文本*、TopK 滑块（1-50）、相似度阈值滑块（0-1）、分类过滤、标签过滤（多选可创建）、开始检索按钮、启用三级缓存/重排序复选框
  - 常用查询模板：标签列表（点击填入）
  - 右栏结果：统计标签（命中数/缓存/耗时/来源）+ 指标（最高/最低/平均分、命中条数）+ 分段卡（序号分数/缓存/来源/doc_id 标签 + 相关/一般/不相关按钮 + 编辑分段链接 + 内容 + 进度条）
  - 调试信息卡：描述列表（产品ID/TopK/阈值/三级缓存/重排序/查询）
- **关键交互**：检索后逐段提交反馈，可跳转编辑分段。

## /knowledge/chunks — 文本块（分段编辑工作台）
- **组件**：`views/KnowledgeWorkspace/ChunkManagement.vue` + `ChunkEditor.vue`
- **布局**：提示 + 文档选择器 + 分段表格 + 编辑/拆分弹窗。
- **UI 元素**：
  - 文档选择器：文档下拉（可搜）、刷新分段按钮、分段计数信息
  - 表格列：序号、内容（点击编辑）、字符数、Tokens、向量（已向量化/待重建）、操作（编辑/拆分/删除）
  - 编辑弹窗：警告提示 + 内容文本域 + 统计（字符数/Tokens估算）+ 取消/保存修改
  - 拆分弹窗：提示 + 原内容（禁用）+ 新分段动态列表（可增删）+ 取消/确认拆分
- **关键交互**：选文档加载分段，编辑后失效向量待重建，拆分生成多段。

## /knowledge/feedbacks — 反馈
- **组件**：`views/KnowledgeWorkspace/FeedbackList.vue`
- **布局**：顶部提示 + 筛选卡 + 4 统计卡 + 反馈表格。
- **UI 元素**：
  - 筛选区：产品下拉、评价下拉（相关/一般/不相关）、查询文本搜索框、搜索按钮、刷新按钮
  - 统计卡：总反馈数、相关、一般、不相关（颜色区分）
  - 表格列：ID、评价（标签）、查询文本、关联文档、关联分段、操作员、会话、备注、时间
  - 分页：20/50/100 每页
- **关键交互**：切换筛选条件即时加载，统计按当前列表本地计算。

## /knowledge/tokens — API Token 管理
- **组件**：`views/KnowledgeWorkspace/ApiToken.vue`
- **布局**：左右双栏，左 Token 列表，右创建表单+调用示例。
- **UI 元素**：
  - 提示区：顶部 info 说明；筛选区：产品下拉（可选）+ 刷新按钮
  - 表格列：ID、名称、产品、权限（标签）、状态（启用/已吊销）、使用统计（调用次数+最后）、过期时间（永不过期）、创建人、操作（吊销）
  - 创建表单：名称*（输入）、产品*（下拉）、权限（复选 只读/可写）、过期时间（日期时间）、创建按钮
  - 成功弹窗：名称/产品（只读）、Token（只读+复制按钮）、我已保存关闭
  - 示例卡：curl 代码块
- **关键交互**：创建后弹窗仅显示一次明文 Token，列表吊销需确认。

## /knowledge/external — 外部系统文档接入
- **组件**：`views/KnowledgeWorkspace/ExternalImport.vue`
- **布局**：左右双栏，左导入测试+结果，右 Token 输入+历史任务。
- **UI 元素**：
  - 提示区：info 说明；导入表单：数据源（单选 通用/飞书/Notion/钉钉）、产品（必填下拉）、飞书DocID/Notion PageID/钉钉DocID（按源显隐）、文档数据（JSON文本必填）、同步复选 + 载入模板、提交导入
  - 结果卡：任务号、状态、总数、成功、失败、异步、文档ID、失败明细
  - Token 卡：Token（密码框）+ 提示文字
  - 历史任务：产品筛选下拉 + 任务表格（任务号、来源、状态、总数、成功/失败、时间）
- **关键交互**：提交后展示响应，历史任务按产品筛选刷新。

## /knowledge/statistics — 知识库统计
- **组件**：`views/KnowledgeWorkspace/KnowledgeStatistics.vue`
- **布局**：顶部筛选 + 多卡片（概览/图表/日志）纵向堆叠。
- **UI 元素**：
  - 筛选区：产品下拉（可选）+ 时间范围（近7/30/90天）+ 刷新
  - 指标卡：文档总数、分段总数、总Token、总检索次数
  - 图表卡：索引健康度（已索引/处理中/待处理/失败 + 进度条）、来源类型分布（进度条）
  - 文档统计（标签）：导入趋势（柱）/ 来源分布 / 分类分布 / 热门文档表（标题、检索次数、命中次数、命中率）
  - 检索趋势（柱）、分数分布（柱）；热点查询表（查询、次数、命中率）、检索质量描述（8项）
  - 导入审计日志表：ID、来源、批次号、操作人、状态、耗时、错误信息、时间
  - OpenAPI 同步卡：数据源总数/已启用/同步失败/累计同步
- **关键交互**：切换产品/时间全部重算刷新。

## /knowledge/openapi — OpenAPI 数据源集成
- **组件**：`views/KnowledgeWorkspace/OpenAPIIntegration.vue`
- **布局**：提示+筛选，数据源表格，新建/编辑弹窗+结果弹窗。
- **UI 元素**：
  - 筛选区：产品下拉（可选）+ 刷新 + 新建数据源
  - 表格列：ID、名称、类型、方法、端点、认证、状态（开关）、上次同步（+标签）、累计同步、操作（编辑/测试/立即同步/删除）
  - 新建/编辑表单：名称*、关联产品*、端点URL*、请求方法（单选）、认证方式（下拉）、Token/APIKey/HMACSecret（按类型显隐）、用户名+密码（Basic）、请求模板（POST/PUT）、响应路径、字段映射、定时任务（cron）、启用（开关）；取消+保存
  - 测试弹窗：是否成功、HTTP状态码、响应耗时、响应大小、错误信息、响应预览
  - 同步结果弹窗：数据源ID、状态、获取条目、已导入、已跳过、失败、耗时、错误信息
- **关键交互**：列表开关即时启停，编辑回填，测试/同步弹窗展示结果。

## /system/rag-product-config — RAG 产品配置
- **组件**：`views/RagProductConfig/index.vue`
- **布局**：产品列表 + 配置抽屉（嵌入模型/检索参数/分块策略）。
- **UI 元素**：
  - 表格列：产品名、状态、Embedding 模型、检索类型、操作（配置/删除）
  - 配置抽屉：Embedding 模型（下拉）、分块大小、重叠、TopK、相似度阈值、是否启用重排序、是否启用三级缓存
- **关键交互**：选择产品打开配置抽屉保存。

## /system/rag-account — RAG 账号配置
- **组件**：`views/RagProductConfig/AccountConfig.vue`
- **布局**：账号列表 + 新增/编辑弹窗（API Key / 端点）。
- **UI 元素**：
  - 表格列：账号名、提供商、状态、创建时间、操作（编辑/删除）
  - 弹窗-编辑：账号名*、提供商*（下拉）、API Base URL*、API Key*（密码）、模型名、启用（开关）
- **关键交互**：保存账号供产品配置引用。

## /system/rag-product — RAG 产品管理
- **组件**：`views/RagProductConfig/RagProductManagement.vue`
- **布局**：产品表格 + 新建/编辑弹窗。
- **UI 元素**：
  - 表格列：产品编码、名称、描述、状态（标签）、文档数、操作（编辑/删除）
  - 弹窗-编辑：产品编码*、名称*、描述、状态（开关）、关联账号（下拉）、默认 Embedding（下拉）
- **关键交互**：产品用于知识库挂载与检索隔离。

## /system/rag-overview — RAG 概览
- **组件**：`views/system/RagOverview.vue`
- **布局**：概览卡片 + 各产品健康度表 + 检索监控。
- **UI 元素**：
  - 指标卡：产品数、文档总数、检索 QPS、平均延迟
  - 产品表：名称、文档数、索引状态、QPS、延迟、错误率
  - 检索趋势图（折线：延迟/QPS）
- **关键交互**：实时刷新监控指标。

## /aiContent/list — AI 内容
- **组件**：`views/aiContent/List.vue`
- **布局**：筛选卡 + AI 生成内容表格 + 详情/再生成弹窗。
- **UI 元素**：
  - 筛选区：类型、关联智能体、状态、关键词、搜索/重置
  - 表格列：标题、类型（标签）、来源（智能体/手工）、状态、创建时间、操作（查看/再生成/删除）
  - 弹窗-详情：内容全文 + 元数据
  - 弹窗-再生成：提示词输入 + 模型选择 + 生成
- **关键交互**：查看或基于原提示再生成内容。

## /templateMarket/list — 模板市场
- **组件**：`views/templateMarket/List.vue`
- **布局**：分类 Tab + 模板卡片网格 + 详情/下载弹窗。
- **UI 元素**：
  - 分类 Tab：话术/流程/SOP/知识库…
  - 筛选区：关键词、排序
  - 卡片网格：封面/名称/作者/下载数/标签 + 操作（预览/使用）
  - 弹窗-详情：模板预览 + 描述 + 使用/收藏按钮
- **关键交互**：点击"使用"将模板导入到当前工作区。

---

# 六、数据分析（analytics）

## /dashboardScreen/list — 营销数据大屏
- **组件**：`views/dashboardScreen/List.vue`
- **布局**：深色全屏大屏，顶部标题栏 + 6 张 KPI 卡 + 多行 ECharts 图表卡片（折线/饼/柱/漏斗）+ 实时活动列表，无筛选区。
- **UI 元素**：
  - 标题区：标题"营销数据大屏"、当前时间、全屏/退出全屏按钮
  - 指标卡：KPI 卡（标签、数值、较昨日涨跌趋势，动态渲染）
  - 图表卡：营销趋势（近30天折线：访问/线索/转化）、渠道分布（饼图）、用户来源 TOP5（横向柱）、漏斗分析（漏斗）、地区分布（柱状）、转化率对比（本周/上周柱）
  - 实时活动：图标+文本+时间滚动列表
- **关键交互**：5 秒定时刷新时间与实时活动，支持全屏切换与窗口缩放自适应。

## /conversionFunnel/list — 转化漏斗
- **组件**：`views/conversionFunnel/List.vue`
- **布局**：头部卡片 + 4 张统计卡 + el-tabs 四标签页（阶段定义表/漏斗可视化/流失分析表/趋势表），含新增阶段弹窗。
- **UI 元素**：
  - 头部：标题、副标题、新增阶段按钮、刷新按钮
  - 指标卡：总进入数、最终转化数、整体转化率、平均流失率
  - 筛选区：转化率统计页日期范围选择器；时间趋势页日/周/月单选组
  - 表格列：阶段定义（顺序、阶段名称、阶段标识、说明、当前人数、操作[编辑/删除]）；流失分析（阶段、进入人数、流失人数、流失率标签、平均停留、主要流失原因、流失率分布进度条）；趋势（时间、进入数、转化数、转化率进度条、平均转化时长）
  - 漏斗可视化：各阶段名称/人数/转化率/流失率色块
  - 弹窗-新增阶段：阶段名称、阶段标识、顺序（数字1-20）、说明（文本域）；按钮 取消/确定
- **关键交互**：切换标签懒加载数据，新增/编辑阶段弹窗校验提交，删除二次确认，刷新重载全部。

## /aiProductivity/list — AI 生产力
- **组件**：`views/aiProductivity/List.vue`
- **布局**：筛选卡 + 指标卡（会话数/AI 接管率/节省工时/满意度）+ 趋势图 + 坐席排行表。
- **UI 元素**：
  - 筛选区：时间范围、渠道、坐席
  - 指标卡：AI 会话占比、平均响应时长、人工介入率、客户满意度
  - 图表：AI 调用趋势（折线）、坐席效率对比（柱状）
  - 表格列：坐席、处理会话数、AI 辅助次数、节省工时、满意度
- **关键交互**：切换筛选刷新全部指标与图表。

## /customReport/list — 自定义报表
- **组件**：`views/customReport/List.vue`
- **布局**：报表列表 + 新建/编辑报表弹窗（字段选择 + 图表类型）。
- **UI 元素**：
  - 表格列：报表名、数据源、图表类型、创建人、更新时间、操作（查看/编辑/删除）
  - 弹窗-编辑：报表名*、数据源*、维度字段（多选）、指标字段（多选）、图表类型（表格/折线/柱状/饼）、筛选条件
- **关键交互**：保存后生成可视化报表。

## /abExperiment/list — A/B 实验
- **组件**：`views/abExperiment/List.vue`
- **布局**：实验列表 + 新建/编辑弹窗 + 结果对比抽屉。
- **UI 元素**：
  - 表格列：实验名、状态（进行中/已结束）、变体数、流量分配、开始/结束时间、操作（详情/停止）
  - 弹窗-编辑：实验名*、目标指标*、变体配置（名称/流量%）、人群定向
  - 结果抽屉：各变体关键指标对比（表格 + 柱状图）+ 显著性
- **关键交互**：创建实验并分配流量；结束后查看显著性结果。

## /churnPrediction/list — 流失预测
- **组件**：`views/churnPrediction/List.vue`
- **布局**：高危客户列表 + 预测详情抽屉（风险分 + 因子）。
- **UI 元素**：
  - 筛选区：风险等级、时间、渠道
  - 表格列：客户、风险分（进度条）、等级（标签）、主要流失因子、最近活跃、操作（详情/干预）
  - 抽屉：风险因子权重、历史行为、建议干预动作
- **关键交互**：查看预测详情并触发干预（如发优惠券/专属客服）。

## /customerJourney/dashboard — 客户旅程
- **组件**：`views/customerJourney/Dashboard.vue`
- **布局**：旅程阶段桑基/漏斗图 + 阶段明细表 + 时间线。
- **UI 元素**：
  - 筛选区：时间范围、客群
  - 图表：旅程流转（桑基图/漏斗）、各阶段转化（折线）
  - 表格列：阶段、进入人数、转化率、平均停留
  - 时间线：关键触点事件流
- **关键交互**：筛选后重算旅程流转与转化。

---

# 七、系统设置（system）

## /system/config — 系统基础配置
- **组件**：`views/system/Config.vue`
- **布局**：单卡片内分区表单，顶部标题+保存按钮，卡片加载有 loading。
- **UI 元素**：
  - 表单：站点名称（输入）、网站URL（输入）、站点Logo URL（输入）、主题色（取色器）、SEO关键词（输入）、SEO描述（多行）、客服电话（输入）、客服邮箱（输入）、ICP备案号（输入）、公安备案号（输入）、用户注册（开关）、邮件营销（开关）、RAG智能体（开关）、维护模式（开关）、用户数（只读"无限制"）、文件上传大小MB（数字 1-1024）；按钮 保存配置（主,loading）
- **关键交互**：挂载加载配置，点保存校验后提交并刷新；无字段必填。

## /system/obs-config — 对象存储配置
- **组件**：`views/system/ObsConfig.vue`
- **布局**：卡片表格列出配置，顶"新增配置"按钮；弹窗表单新增/编辑。
- **UI 元素**：
  - 表格列：配置名称、服务商（标签）、存储桶、节点域名、默认（标签）、状态（标签）、操作（测试/编辑/设为默认[非默认时]/删除）
  - 弹窗-表单（带*必填）：配置名称*、服务商*（下拉 七牛云/阿里云OSS/腾讯云COS/AWS S3/本地存储）、存储桶名称*、Access Key*、Secret Key*（密码）、节点域名*（非本地）、存储区域*（AWS）、存储路径*（本地）、CDN域名（可选）、状态（单选 启用/禁用）、备注（多行）；按钮 取消/确定
- **关键交互**：选服务商联动默认值；测试/设为默认/删除（确认）行操作；提交校验后创建或更新。

## /system/material-library — 素材库
- **组件**：`views/system/MaterialLibrary.vue`
- **布局**：卡片含筛选栏+素材网格+分页；上传弹窗、分类管理弹窗。
- **UI 元素**：
  - 筛选区：分类（下拉可清）、类型（下拉 图片/视频/音频/文档）、关键词（输入）；按钮 搜索、重置。顶部：上传素材（主）、新建分类（成功）
  - 网格项：预览图/图标、名称、大小/类型/使用次数；悬浮操作：复制链接、删除
  - 分页：每页 12/24/48/96，total/sizes/prev/pager/next/jumper
  - 上传弹窗：选择分类（下拉）、上传文件（拖拽多文件,≤10MB）；按钮 取消、开始上传
  - 分类弹窗：分类列表（编辑/删除）、新增分类（输入+保存/取消）
- **关键交互**：筛选/分页刷新网格；上传需选分类与文件；删除/删分类均确认。

## /system/monitor — 系统监控
- **组件**：`views/system/Monitor.vue`
- **布局**：服务器指标卡（CPU/内存/磁盘/网络）+ 进程状态表 + 实时折线图。
- **UI 元素**：
  - 指标卡：CPU 使用率、内存、磁盘、网络吞吐、在线用户数
  - 图表：CPU/内存实时折线（ECharts）
  - 表格列：服务名、状态、CPU、内存、运行时间、操作（重启）
- **关键交互**：定时刷新监控数据。

## /system/guide — 使用指南
- **组件**：`views/system/Guide.vue`
- **布局**：步骤卡片 + FAQ（类似邮件指南）。
- **UI 元素**：
  - 卡片列表：初始化 / 配置渠道 / 知识库 / 智能体 / 触达，含"前往操作"链接
  - FAQ：常见问题折叠面板
- **关键交互**：点击卡片跳对应路由。

## /domainPool — 域名池
- **组件**：`views/domainPool/List.vue`
- **布局**：筛选卡 + 域名列表表格 + 新增/编辑弹窗。
- **UI 元素**：
  - 筛选区：域名、状态、分组、搜索/重置
  - 表格列：域名、分组、状态（标签）、健康（标签）、到期时间、操作（编辑/删除/检测）
  - 弹窗-编辑：域名*（URL）、分组、状态（开关）、备注
- **关键交互**：检测按钮触发健康检查。

## /teamUser/list — 团队成员
- **组件**：`views/teamUser/List.vue`
- **布局**：成员列表 + 新增/编辑弹窗（角色分配）。
- **UI 元素**：
  - 筛选区：姓名/账号、角色、状态、搜索/重置
  - 表格列：头像、姓名、账号、角色（标签）、状态、最后登录、操作（编辑/禁用/删除）
  - 弹窗-编辑：姓名*、账号*、角色*（下拉）、状态（开关）、密码（新增时）
- **关键交互**：编辑成员角色与状态。

## /teamUser/role — 角色权限
- **组件**：`views/teamUser/Role.vue`
- **布局**：角色列表 + 权限树配置抽屉。
- **UI 元素**：
  - 表格列：角色名、描述、成员数、操作（编辑/删除）
  - 权限树：菜单/操作权限勾选（el-tree 带 checkbox）
  - 弹窗-编辑：角色名*、描述、权限树勾选、保存
- **关键交互**：勾选权限保存后生效。

## /platformAccount/list — 平台账号
- **组件**：`views/platformAccount/List.vue`
- **布局**：平台账号列表 + 绑定/解绑操作。
- **UI 元素**：
  - 表格列：平台名、账号、绑定状态（标签）、绑定时间、操作（绑定/解绑/刷新）
  - 弹窗-绑定：账号*、密钥*（密码）、回调域名
- **关键交互**：绑定外部平台账号用于数据同步。

## /payment/list — 支付记录
- **组件**：`views/payment/List.vue`
- **布局**：支付流水表格 + 详情弹窗。
- **UI 元素**：
  - 筛选区：订单号、状态、时间、搜索/重置
  - 表格列：流水号、订单、金额、渠道、状态（标签）、完成时间、操作（详情/退款）
  - 弹窗-详情：支付信息描述列表 + 回调日志
- **关键交互**：退款二次确认。

## /payment/config — 支付配置
- **组件**：`views/payment/Config.vue`
- **布局**：多支付商配置段（微信/支付宝/Stripe）。
- **UI 元素**：
  - 表单：启用支付商（多选）、商户号、API Key（密码）、回调地址、启用（开关）
  - 按钮：保存配置
- **关键交互**：保存全量支付配置。

## /integration/list — 集成管理
- **组件**：`views/integration/List.vue`
- **布局**：集成卡片/列表 + 配置弹窗。
- **UI 元素**：
  - 表格列/卡片：集成名、类型（标签）、状态（开关）、操作（配置/禁用）
  - 弹窗-配置：按集成类型动态表单（endpoint/Key/Secret/Webhook 等）
- **关键交互**：开关启停；配置保存。

## /operationLog/list — 操作日志
- **组件**：`views/operationLog/List.vue`
- **布局**：筛选卡 + 日志表格。
- **UI 元素**：
  - 筛选区：操作员、模块、动作、时间范围、搜索/重置
  - 表格列：时间、操作员、模块、动作、对象、IP、详情（可展开）
  - 分页
- **关键交互**：筛选/分页查询日志。

## /backup/list — 备份管理
- **组件**：`views/backup/List.vue`
- **布局**：备份列表 + 创建/恢复按钮。
- **UI 元素**：
  - 表格列：备份名、类型（全量/增量）、大小、状态、时间、操作（恢复/下载/删除）
  - 按钮：立即备份、定时策略配置
- **关键交互**：创建备份；恢复到指定时间点（二次确认）。

## /securityAudit/list — 安全审计
- **组件**：`views/securityAudit/List.vue`
- **布局**：安全事件表格 + 风险概览卡。
- **UI 元素**：
  - 指标卡：异常登录数、高危操作数、封禁账号数
  - 筛选区：事件类型、级别、时间、搜索/重置
  - 表格列：时间、类型、级别（标签）、来源IP、账号、描述、状态、操作（处理/忽略）
- **关键交互**：处理或忽略安全事件。

---

# 八、独立页面（非菜单内）

## /setup — 系统初始化向导
- **组件**：`views/setup/InitSetup.vue`
- **布局**：居中卡片式三步向导（阅读协议 → 创建超管 → 完成），顶部 el-steps 进度条。
- **UI 元素**：
  - 第一步 阅读协议：标题"软件使用声明" + 3 条声明列表 + 复选框"我已阅读并同意以上使用条款"；按钮[下一步]（未勾选禁用）
  - 第二步 创建超级管理员：表单字段 超管账号*（3-20位字母数字下划线）、超管密码*（8位含大小写+数字,带提示）、确认密码*、手机号（选填）、邮箱（选填）、姓名（选填）；按钮[上一步][创建并完成初始化]（loading）
  - 第三步 完成：el-result 成功提示"系统初始化已完成" + 描述列表（超管账号、提示）；按钮[前往登录]
- **关键交互**：勾选协议后才能下一步；提交调 /api/system/init-admin 建超管并上报；挂载时查 init-status，已初始化直接跳 /login。

## /login — 登录页
- **组件**：`views/Login.vue`
- **布局**：左右分栏（左侧紫蓝渐变品牌展示面板，右侧白色登录卡片），窄屏隐藏左侧。
- **UI 元素**：
  - 左侧面板：品牌标识+应用名、主标题"全域获客·转化·复购"、描述文案、4 条特性列表（私有化/全渠道/RAG/7×24）、版权页脚
  - 右侧登录表单：标题"欢迎"+应用名副标题；字段 用户名*（User图标）、密码*（Lock图标,可显密）；按钮[登录]（全宽,loading）；底部免责声明文字
- **关键交互**：支持回车提交；校验通过后调 usersApi.login，存 token/user，首次登录（mustChangePassword）跳 /change-password，否则跳 /。

## /profile — 个人中心
- **组件**：`views/Profile.vue`
- **布局**：个人信息卡片 + 账号安全区 + 偏好设置。
- **UI 元素**：
  - 基本资料：头像、姓名（输入）、手机号、邮箱、部门；按钮 保存
  - 账号安全：修改密码入口、绑定邮箱/手机状态、两步验证（开关）
  - 偏好：语言、时区、通知开关
- **关键交互**：保存个人资料；跳转改密。

## /notifications — 通知中心
- **组件**：`views/Notifications.vue`
- **布局**：通知列表（已读/未读分组）+ 标记已读操作。
- **UI 元素**：
  - 筛选区：类型、已读/未读、全部已读按钮
  - 列表项：图标、标题、摘要、时间、未读徽标、点击查看
  - 操作：标记已读、删除
- **关键交互**：查看/批量标记已读。

## /chat/embed — 嵌入客服窗
- **组件**：`views/chat/embed/Index.vue`
- **布局**：全屏浮层聊天窗（用于网站嵌入），含消息区 + 输入框 + 转人工。
- **UI 元素**：
  - 头部：渠道名/在线状态、关闭按钮、最小化
  - 消息区：访客/客服气泡 + 时间
  - 输入区：文本输入、发送、快捷问题卡片
  - 转人工：按钮触发接入坐席
  - 文件上传（若启用）
- **关键交互**：访客发送消息，命中智能体自动回复；可转人工。

## NotFound — 404
- **组件**：`views/NotFound.vue`
- **布局**：居中插画 + 提示"页面不存在" + 返回首页按钮。
- **UI 元素**：状态码、描述文案、[返回首页] 按钮。
- **关键交互**：点击返回 /。

## /change-password — 修改密码
- **组件**：`views/ChangePassword.vue`
- **布局**：居中卡片表单。
- **UI 元素**：字段 原密码*、新密码*（强度校验）、确认新密码*；按钮[提交]（loading）。
- **关键交互**：校验一致后提交；成功后跳登录或首页。

## /forgot-password — 找回密码
- **组件**：`views/ForgotPassword.vue`
- **布局**：步骤表单（验证身份 → 重置密码）。
- **UI 元素**：
  - 验证：账号*、验证码*（短信/邮箱获取）、[下一步]
  - 重置：新密码*、确认密码*、[提交]
- **关键交互**：获取验证码 → 校验 → 重置密码。

---

## 逐页面 API 链路核查（2026-07-22）

> 方法：对 `src/views` 下 110 个菜单对应视图，逐一读源码，核对「是否调用 API → 前端 `@/api/*` 端点的真实 method/URL → 后端 `user-server` 路由是否真实存在 → 响应是否按拦截器规则正确渲染」。判定规则：**请求拦截器成功分支 `return data.data`，业务 `res` 即后端 `data` 内层载荷（对象或数组），无 `.code`/`.data` 子字段**；正确写法为列表 `toList(res)`、单对象直接用 `res`；凡写 `res.data`/`res.code===0`/`response.data`/`if(res.success)` 均属错误。`el-table :data` 须绑数组。
> 全部为只读核查，未改动代码。下文 `❌`=功能不可用（恒空/404/字段错），`⚠️`=脆弱写法当前碰巧渲染但违反规则易回归，`ℹ️`=轻微/非 API 运行时问题。

### 总体结论
| 级别 | 数量 | 说明 |
|---|---|---|
| ❌ 严重（数据恒空 / 404 / 字段错） | 13 处页面/模块 | 列表或关键数据完全不显示，功能不可用 |
| ⚠️ 中等（脆弱响应处理） | 13 处 | 用 `res.data`/`res?.data` 兜底，当前渲染但违反规则，后端一旦改包即崩 |
| ℹ️ 轻微 / 非 API 运行时 bug | 5 处 | 拼写/变量/语义错位，非接口响应问题 |
| ✅ 已确认无问题 | 多数页面 | 见末尾"已确认 OK"清单 |

### 一、❌ 严重问题（建议优先修复）

| # | 页面（文件:行） | 问题 | 影响 |
|---|---|---|---|
| 1 | `platformAccount/List.vue:320` | `accountList.value = res.list \|\| []`；后端 `GetAccounts` 返回 `response.Success(ctx, accounts)`（`accounts` 为切片，经拦截器 `res` 即数组本身），`res.list` 恒 `undefined` | 平台账号列表**永远为空**（已亲手核实后端 + 前端） |
| 2 | `unifiedMessage/List.vue`（api `/api/messages`） | 前端调 `/api/messages`、`/api/messages/:id`、`/api/messages/:id/replies`；后端仅注册 `/api/unified-messages*`（`frontend_aliases.go`），**无 `/messages` 别名** | 统一消息列表/详情/回复 **全部 404 加载失败**（已核实） |
| 3 | `userSegment/List.vue:262,263,320` | `res.data \|\| []`（`getUserSegments`/`getSegmentUsers`/`getSegmentStats`） | 用户分群列表、分群内用户列表、统计**恒空** |
| 4 | `marketingFlow/List.vue:160` | `flows.value = res.data \|\| []` | 营销流程列表**恒空** |
| 5 | `batchOperation/List.vue:228,242` | `res.data \|\| []`（`refreshTools`/`refreshHistory`） | 批量工具列表、执行历史**恒空**；L301 详情 `res.data \|\| row` 回退行数据 |
| 6 | `customerService/QuickReply.vue:134,135` | `const replies = rRes.data \|\| rRes`；`const cats = cRes.data` | 客服快捷回复列表、分类列表**恒空** |
| 7 | `aiContent/List.vue:194` | `historyList.value = res.data \|\| []` | AI 内容历史记录列表**恒空** |
| 8 | `xiaohongshuCard/Stats.vue:93,110` + `CardStats.vue:279` | 用 `overallStats.top_cards`/`recent_activities`/`res.daily_stats`，后端返回 `popularCards`/`recentActivity`/`dailyStats`（snake→camel 不匹配） | 热门卡片表、近期活动表、每日活动表**空白** |
| 9 | `aiProductivity/List.vue` | 4 张表 `toList(res)` 但后端 `GetAIMetric` 单接口忽略 `metric` 参数只返回标量；概览字段名不匹配 | 4 表恒空 + 概览指标全 0 |
| 10 | `churnPrediction/List.vue` | ①列绑 `user_id`/`risk_score` 等 snake，后端返回 `userId`/`riskScore`（camel）列全空；②`stats` 后端返回**数组**前端当对象取 `highRisk` 恒 0；③详情 `res.data \|\| row`；④`interveneUser(row.userId)` 参数缺失 | 列表列空、风险概览恒 0、详情错、干预无参数 |
| 11 | `system/RagOverview.vue:259` | `productsRes?.list`；后端 `GetMerchantRagProducts` 返回 `items` 字段 | RAG 产品统计**恒 0** |
| 12 | `operationLog/List.vue:183` | `logDetail.value = res.data` | 操作日志详情**空白**（应直接用 `res`） |
| 13 | `integration/List.vue:278` + `:230` | `if (res?.success)` 误判连接失败；`stats` 字段名不匹配 | 集成连接状态误报失败、统计表恒空 |

### 二、⚠️ 中等问题（脆弱写法，当前渲染但违反规则）

| 页面（文件:行） | 写法 | 风险 |
|---|---|---|
| `clue/Statistics.vue:45` | `res.data \|\| res` | 统计对象字段取不到 |
| `tagSegmentation/List.vue:276,289,302,315` | `res?.data \|\| res` | 标签列表/详情/用户/统计取不到 |
| `objection/List.vue:272` | `res.data \|\| res` | 异议列表取不到 |
| `persona/List.vue:348,365` | `res.data \|\| res` | 画像报告/分群数据取不到 |
| `customerSession/List.vue:903` | `rRes.data \|\| rRes \|\| []` 且 `.catch(() => ({ data: [] }))` 制造假 `data` | 会话列表脆弱，catch 静默伪造空数据 |
| `aiAgent/Edit.vue:464` | `res?.data \|\| res?.list` | 编辑加载脆弱 |
| `llmRouting/List.vue:268,281,294` | `res?.data \|\| res` | 路由列表/详情/测试取不到 |
| `customerSession/List.vue:596,621,776` | `res?.data?.list` | 多列依赖深层 data，后端一改即崩 |
| `dashboardScreen/List.vue:132,140` | `res?.data?.data \|\| res?.data \|\| res` | 三层兜底，脆弱 |
| `customerJourney/Dashboard.vue:273,274` | `res.data \|\| res`；`total_customers` vs `TotalCustomers` | 旅程数据/总量取不到或恒 0 |
| `teamUser/List.vue:226-227` | `mRes.data?.list \|\| mRes.data \|\| mRes.list` | 成员列表脆弱 |
| `securityAudit/List.vue` | `row.passed` vs 后端 `passed_count` | 通过计数**恒 0** |
| `messageHub/List.vue:596` | `if (res && res.duplicate)` | 幂等提示 `duplicate` 字段恒 false，提示缺失（非崩溃） |

### 三、ℹ️ 轻微 / 非 API 运行时问题

| 页面（文件:行） | 问题 |
|---|---|
| `telegram/account.vue:190` | `kw` 未定义 `ReferenceError`（搜索交互 bug，非接口响应） |
| `feishu/account.vue:405`（及多处） | `bidingForm.value.account_id` 拼写错（应为 `bindingForm`），运行时报错 |
| `domainPool/List.vue:274` | 提示文案用 `res.message` 而结构为 `msg`，提示文案 undefined（轻微） |
| `payment/List.vue` | 复用 `OrderApi`（语义错位），但接口可用，非崩溃 |
| `churnPrediction/List.vue` | `runChurnPrediction` 调 `/api/user-segment/rfm/calculate`，后端路由**未确认**（需人工核对） |

### 四、已确认 OK（代表性，非全部）
- **workspace**：`wecomAccount`（账号健康度）✅
- **customer**：`clue/List`、`customer360/List`、`customerEvent/List`、`oneid/List` ✅（`oneid/Conflicts` 为已知桩，无接口）
- **reach**：`whatsapp`（drafts/jobs/account）、`douyin*`、`kuaishou*`、`xianyu*`、`tiktok*`、`sms*`、`email*`、`community`、`shortLink`、`livecode` ✅
- **knowledge**：`KnowledgeWorkspace`（10 子页）、`RagProductConfig`（3 子页）、`templateMarket` ✅
- **analytics**：`conversionFunnel`、`customReport`、`abExperiment` ✅
- **system**：`Config`、`Guide`、`MaterialLibrary`、`Monitor`、`ObsConfig`、`backup` ✅；`order` 模块 pay 路由**实际可用**（后端同时注册 `/order/:id/pay` 与 `/orders/:id/pay`，前端 `/api/order/:id/pay` 命中前者，原代理误报，已更正）
- **aiAgent**：除上述中等项外，`aiAgent/List`、`unifiedInbox`、`intentRecognition`、`dialogueMemory`、`reachPipeline`、`sopAgent`、`scriptTemplate`、`chatChannel`、`customerService`（agentStatus/sessionTag/aiSuggestion）列表渲染正常 ✅

### 五、重点核实记录（亲手核对，防误报）
1. **platformAccount**：后端 `setupPlatformAccountRoutes` → `GET /platform-accounts` → `GetAccounts` → `response.Success(ctx, accounts)`（`accounts` 为 `[]*model.PlatformAccount` 切片）。拦截器 `return data.data` 后前端 `res` = 切片数组，写 `res.list` 必为 `undefined` → **列表恒空**，确属 bug。
2. **unifiedMessage**：后端 `frontend_aliases.go` 仅注册 `/unified-messages*`；前端 `unifiedMessage.js` 调 `/api/messages*` → **404**，确属 bug。建议前端改为 `/api/unified-messages` 或后端补 `/messages` 别名。
3. **order pay**：后端 `content_routes.go` 与 `frontend_aliases.go` 均注册 `/order/:id/pay`（`POST`），前端 `/api/order/:id/pay` 命中 → **正常**，非 bug。

> 下一步：以上 ❌ 严重项（尤其 #1 platformAccount、#2 unifiedMessage）会直接导致对应页面空白，建议优先修复为 `toList(res)` / 直接用 `res`，并修正 unifiedMessage 端点前缀。

---

> 说明：部分路由（如 `/chatChannel/edit/:id`、`/douyin-card-stats/:id`、`/chat/embed/*`）含动态参数，已在对应父级菜单下说明。所有表格均带分页，操作列按钮大多带二次确认（删除/停止/吊销等）。必填字段以 `*` 标注。

---

## 九、修复执行记录（2026-07-22，全量一次性修复）

对「逐页面 API 链路核查」章节列出的全部 ❌严重 / ⚠️中等 / ℹ️轻微 问题已逐一修复（除标注「无需改」者）。核心规则：`request` 拦截器 `return data.data`，业务拿到的 `res` 即后端载荷（对象/数组），无 `.data`/`.code` 子字段；列表用 `toList(res)` 或 `res.list`/`res.items`，单对象直接用 `res`。

### ❌严重（13 项）
1. `platformAccount/List.vue`：`accountList.value = res.list || []` → `res || []`（后端返回切片数组）。
2. `unifiedMessage`：`src/api/unifiedMessage.js` 端点 `/api/messages*` → `/api/unified-messages*`；模板列/详情字段对齐后端（`message_id`/`sender_name`/`chat_id`/`content_type`，状态用 `pending/processing/...`）。
3. `userSegment/List.vue`：`segRes.data`/`statsRes.data`/`res.data`（`segments`/`stats`/`segmentUsers`）→ 直接用 `res`/`segRes`/`statsRes`。
4. `marketingFlow/List.vue`：`flows.value = res.data || []` → `res || []`。
5. `batchOperation/List.vue`：`tools`/`histories`/`taskDetail` 的 `res.data` → `res`（detail 用 `res || row`）。
6. `customerService/QuickReply.vue`：`rRes.data`/`cRes.data` → 归一 `Array.isArray(...)?...:(?.list||[])`。
7. `aiContent/List.vue`：`history.value = res.data || []` → `res || []`。
8. `xiaohongshuCard/Stats.vue` + `CardStats.vue`：后端返回 camelCase（`totalCards`/`popularCards`/`dailyStats`/`recentActivity`，`DailyStat.date`/`view`），在 `loadData`/`loadCardStats` 中映射到模板使用的 snake_case（`total_cards`/`top_cards`/`daily_stats`/`recent_activities`/`view_trend`，`view` 非 `views`）。
9. `aiProductivity/List.vue`：`overview.value = res?.data || res` → 显式映射后端 `GetReport` 的 snake_case 聚合字段到 camelCase（`total_conversations`→`totalConversations` 等）。*注：4 张明细表（会话/转化/响应/销售/满意度）后端 `GetReport` 仅返回聚合数、无逐行数据，属后端能力缺口，前端无法凭空渲染，需后端新增按 metric 返回数组的接口。*
10. `churnPrediction/List.vue`：列/详情字段对齐后端 snake_case（`user_id`/`churn_score`/`churn_risk`/`last_activity_at`/`predicted_at`/`risk_factors`，`risk_factors` 为 JSON 字符串已解析为数组）；`stats` 由预测列表按 `churn_risk` 聚合；详情 `res.data || row` → `res`；`interveneUser` 请求体改为 `{warning_id, intervention_type, note}`。
11. `system/RagOverview.vue`：`productsRes?.list` → `productsRes?.items`（后端 `GetMerchantRagProducts` 返回 `items`）。
12. `operationLog/List.vue`：`currentLog.value = res.data` → `res`。
13. `integration/List.vue`：列表 `toList(iRes)` → `iRes?.accounts || toList(iRes)`；列字段对齐（`account_name`/`platform`/`status`/`last_sync_at`）；`testConnection` 的 `res?.success` → `res?.status === 'ok'`；`stats` 由账户列表聚合（enabled/disabled/error/totalCalls）。移除未使用的 `getIntegrationStats` 导入。

### ⚠️中等（13 项）
- `clue/Statistics.vue`、`tagSegmentation/List.vue`、`objection/List.vue`、`persona/List.vue`、`customerSession/List.vue`（会话列表/消息/标签/快捷回复）、`aiAgent/Edit.vue`、`llmRouting/List.vue`（3 处）、`dashboardScreen/List.vue`（双重 `.data`）、`customerJourney/Dashboard.vue`、`teamUser/List.vue`：统一去掉无效的 `res?.data`/`res.data` 兜底，改为直接用 `res`（或 `res?.list`/`Array.isArray(res)?res:...`）。
- `securityAudit/List.vue`：`row.passed` → `row.passed_count`（后端 snake_case，原恒 0）。
- `messageHub/List.vue`：L596 `res && res.duplicate` 本就正确，无需改。

### ℹ️轻微（5 项）
- `telegram/account.vue`：复核 `kw` 已在计算属性内正确定义，无需改（原报告误报）。
- `feishu/FeishuAccount.vue`：复核全文件均为 `bindingForm`（正确拼写），原报告 `bidingForm` 误报，无需改。
- `domainPool/List.vue`：`res.message` → `res.msg`（后端返回 `msg`）。
- `payment` 复用 `OrderApi`：属语义命名，非渲染 bug，未改。
- `churnPrediction` `runChurnPrediction` → `POST /api/user-segment/rfm/calculate`：已确认后端 `business_routes.go` 注册该路由，可用，未改。

### 验证
- 所有改动文件经 IDE lint 检查 **0 错误**。
- 已用 node 22.12.0 跑通 `npm run build`（0 编译错误）与 `npx vitest run`（184 项全过）。
- 高风险字段映射已逐项比对后端确认一致：xiaohongshu（`totalCards`/`popularCards`/`dailyStats`/`recentActivity` 等 camelCase DTO、`DailyStat.view` 单数）、churnPrediction（`risk_factors` 为 JSON 字符串已 `JSON.parse`、`user_id`/`churn_score`/`churn_risk`/`last_activity_at`/`predicted_at` 为 snake_case）、integration（`status` 为 int 1/0，列表接口直接返回 `[]*IntegrationAccount` 数组；`TestIntegration` 返回 `{"status":"ok"}` 字符串，匹配前端 `res?.status === 'ok'`）。
- `unifiedMessage` 端点 `/api/unified-messages` 已在 `frontend_aliases.go` 注册（`GET /unified-messages` → `GetMessages`），与前端一致。
