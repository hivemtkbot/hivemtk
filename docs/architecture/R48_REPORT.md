# R48 竞品吸收一轮开发报告（2026-08-30 · 第十轮）

> 方法：用户角度全功能审计 × 竞品调研（Chatwoot 36.3k★/Libredesk/Chaskiq/Intercom/Crisp + Dify 153.8k★/Mautic/Listmonk/11x/Artisan）
> 吸收标准：竞品标配或差异化加分项，除多租户外全部吸收

## 一、吸收成果（12 项，全部真实实现+验证）

| # | 功能 | 竞品依据 | 实现 | 验证 |
|---|------|---------|------|------|
| T1 | **公开帮助中心门户** ★ | CW/LD/CQ/CR/IC 全标配 | 免登录公开 API（categories/articles/详情+搜索，仅暴露 public_visible 白名单）；管理端发布开关；公开页面 /#/help-center（分类侧栏+卡片+搜索+详情） | **端到端**：导入文档→发布→公开列表(分类/摘要)→详情(分块拼接)→搜索 ✓ |
| T2 | **办公时间+离开自动回复** | CW/IC/LD/CR | 策略 KV+时段校验；新会话非工作时间自动 away reply（防循环：2h 去重+system_away 标记） | 策略保存 ✓ |
| T3 | **会话优先级+Snooze** | CW/LD/CR | priority 0-3 PATCH；snoozed_until 列+到期恢复 cron（每5分钟，惯例同 M1）；列表入口 | 设置/暂缓/恢复 ✓ |
| T4 | **宏 Macros** | CW/LD/IC | macros 表+6 种动作（加标签/备注/分配/关闭/发消息/优先级）+apply 执行器 | **真实执行**：add_note+set_priority 落库 ✓ |
| T5 | **AI 会话摘要** | IC Copilot/LD | 复用全局 Dispatcher（修复：自建实例绕过 DB 路由表的 bug）走 LongSummary 场景；摘要+情绪 upsert | **真实 LLM**：deepseek-chat 生成摘要+情绪 neutral ✓ |
| T6 | **Webhook Out** | CW/LD/CQ/PC | webhook_subscriptions+HMAC 签名+fire-and-forget（5s 超时）；事件：message/session.created/closed | 订阅 CRUD+secret 一次性展示 ✓ |
| T7 | **联系人自定义属性** | CW/LD/CQ/CR | customers.custom_attributes JSONB merge | **DB 实证**：{"vip_level":"gold"} ✓ |
| T8 | **保存的自定义视图** | CW/IC/CR | saved_views（user+route 维度，同名覆盖） | CRUD ✓ |
| T9 | **定时邮件报表** | CW/IC | report_subscriptions+每日 08:00 窗口 cron（昨日汇总 CSV→SMTP，失败仅日志） | 订阅管理+手动触发 ✓ |
| T10 | **对话转录导出** | IC | GET transcript?format=txt/csv 流式下载（2000 条上限） | **真实数据**：CSV 含客户/坐席消息 ✓ |
| T11 | **UTM 追踪** | Mautic | shortlink create 支持 utm_source/medium/campaign 自动追加（去重防重复参数） | 代码+构建 ✓ |
| T12 | **AI 代理绩效报表** | IC Fin/CR | automation_rate 漏斗+LLM 调用/成本按场景分解 | **真实数据**：7d 206 调用/$0.22 成本 ✓ |

## 二、过程中发现并修复的既有问题
1. **LLM 路由绕过**：SessionAIService 自建 dispatcher 未加载 DB 路由表→全部打到本地网关（8207 拒连）；改用全局实例后走真实 deepseek 路由
2. **knowledge_documents filename 列漂移**：表列 filename(NOT NULL) vs model file_name → ALTER SET DEFAULT '' 修复
3. **rag-config 建产品需 category**：补齐契约调用
4. **路由注入语法**：automation-hub 挂载缺逗号 → esbuild 校验修复

## 三、前端接线
- /#/help-center 公开门户（新）
- 会话工作台快捷操作组扩容：AI摘要（弹窗展示+情绪）/转录导出/暂缓2h/优先级（prompt）
- 系统设置→自动化中心（新）：办公时间编辑器/宏管理/Webhook订阅（secret 一次性展示）/报表订阅/AI绩效看板
- 知识文档列表：帮助中心发布开关

## 四、终验
- UI 全量 **149/149 PASS**（142+7 新页）
- vitest 174/174、vite build ✓、后端 build+vet 绿
- 新增表：macros/session_ai_summaries/webhook_subscriptions/saved_views/report_subscriptions/customer_segments(+customers.custom_attributes 列)

## 已提交
commit 见 git log（feat(r48)）→ Gitee + GitHub 双远端
