# R48 竞品吸收任务清单（2026-08-29 · 用户角度审计 × 竞品调研）

> 方法：用户角度全功能审计 × Chatwoot/Intercom/Crisp/Libredesk（客服）+ Dify/Mautic/Listmonk/11x/Artisan（AI销售营销）竞品调研
> 吸收标准：竞品 4 家以上标配或明确差异化加分项，除多租户外全部吸收

## 一、存量核对结论（不重复造）
已有：统一收件箱/标签/批量操作/编辑锁(碰撞检测)/私人备注/@提及/分配(round-robin)/坐席容量/RBAC/快捷回复/KB挂载AI/群发/会话报表/实时监控/CSAT/CSV导出/drip序列/混合编排/AB实验/行为评分/生命周期/去重合并/CRM唤醒/AI撰写/审批门/频控/审计日志/访客识别/事务API/会话Priority字段(缺前端)
真缺口：12 项

## 二、任务清单（12项，一次性开发）

| # | 任务 | 竞品依据 | 明细 |
|---|------|---------|------|
| T1 | **公开帮助中心门户** ★最大缺口 | CW/LD/CQ/CR/IC 全标配 | 公开路由(免登录): /help-center 页(文章列表+分类+搜索+详情)；后端 public API(从 knowledge/faq 数据源查 enabled 文章)；SEO title/meta |
| T2 | **办公时间+离开自动回复** | CW/IC/LD/CR 标配 | system_config_kv 策略{enabled,daily_ranges,away_message}；触达/自动回复管道非工作时间检查→自动 away reply 入队；设置页卡片 |
| T3 | **会话优先级+Snooze 暂缓** | CW/LD/CR | Priority 字段已有→补列表列+筛选+修改入口；新增 snoozed_until 列+PATCH+snoozed 过滤+到期回活跃(cron 已有模式) |
| T4 | **宏 Macros 一键多动作** | CW/LD/IC 标配 | macros 表(name,actions JSON)；apply 端点执行动作序列(加标签/内部备注/分配/关闭/发消息)；会话页下拉 |
| T5 | **AI 会话摘要** | IC Copilot/LD | POST /api/customer-sessions/:id/ai-summary→LLM dispatcher 摘要→存表→返回；会话页按钮 |
| T6 | **Webhook Out 事件订阅** | CW/LD/CQ/PC 标配 | webhook_subscriptions 表(url,events,secret)；message/session 事件发布点→HMAC 签名 fire-and-forget POST；管理 CRUD |
| T7 | **联系人自定义属性** | CW/LD/CQ/CR 标配 | customers.custom_attributes JSONB；PATCH 端点；customer360 展示 |
| T8 | **保存的自定义视图** | CW/IC/CR | saved_views 表(user_id,name,route,filter JSON)；CRUD；列表页应用 |
| T9 | **定时邮件报表订阅** | CW/IC | report_subscriptions 表(report emails schedule)；复用 cron 模式每日08:00 昨日汇总 CSV→SMTP |
| T10 | **对话转录导出** | IC | GET /api/customer-sessions/:id/transcript?format=txt/csv 流式下载 |
| T11 | **UTM 追踪** | Mautic 标配 | shortlink create 支持 utm_source/medium/campaign 自动追加；点击记录归因 |
| T12 | **AI 代理绩效报表** | IC Fin/CR | GET /api/analytics/ai-performance：llm_routing_logs+sop_executions 聚合自动化率/解决漏斗 |

## 三、红队论证
- T1 免登录安全：只暴露 enabled 文章白名单查询，不暴露草稿/内部文档
- T2 away reply 防循环：away 消息本身不再触发 away（发送标记 origin=system_away）
- T3 snooze 到期恢复：复用既有 cron 装配模式，不加新调度器
- T6 webhook 防阻塞：fire-and-forget+超时5s+失败日志（与 exposure/audit 同模式）
- T9 无 SMTP 时：订阅保存成功但发送失败记日志（不阻塞）
- 全部沿用五层架构+doReg 装配模式

## 四、开发批次（一次性完成）
- B1: T1 帮助中心（后端 public API+前端公开页）
- B2: T2+T3 办公时间/离开回复+优先级/snooze
- B3: T4+T5 宏+AI摘要
- B4: T6+T7 Webhook Out+自定义属性
- B5: T8+T9 保存视图+报表订阅
- B6: T10+T11+T12 转录导出+UTM+AI绩效
- B7: 全量测试(api+ui+db+日志)+提交推送
