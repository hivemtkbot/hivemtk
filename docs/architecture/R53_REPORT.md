# R53 业务链闭环报告（2026-08-30 · 第十五轮：吸收完整业务链）

> 方法：三路并行深挖竞品**端到端业务链**（非功能点）：Chatwoot 源码级5链 / Mautic 5.x 文档4链 / Dify 新版文档3链，逐环对照我方代码找断点。

## 一、吸收成果（5条业务链闭环）

### A. 会话生命周期链闭环（Chatwoot 链2/3 对标）✅
| 环节 | 实现 | 实证 |
|------|------|------|
| resolved/closed → CSAT 自动下发 | TriggerCSATOnClose 接入 UpdateSessionStatus 迁移点 | **DB实证**: closed→csat_surveys status=sent triggered_by=auto |
| 自动解决 SLA | auto_resolve KV配置(enabled/hours/tag) + RunAutoResolve cron(10min) | 配置API+超时扫描落库 |
| 访客消息→closed 自动 reopen | ReopenOnInboundMessage 接入 MessageHub.Push(inbound) | **DB实证**: closed→inbound→waiting |

### B. 自动化规则引擎（Chatwoot automation_rules 精简版）✅
- automation_rules + rule_pending_executions 两表
- 事件3种（conversation_created/message_inbound/session_resolved）× 条件6字段4操作符 × 动作7种 + 延迟执行队列（2min cron 复核）
- 事件接线点：CreateSession→conversation_created / MessageHub.Push(inbound)→message_inbound+reopen / UpdateSessionStatus→session_resolved
- **全链实证**：规则(platform=web→set_priority=2+add_tag=r53auto) → 新建web会话 → **priority=2 tags=["r53auto"] run_count=1**

### C. 知识赋能链闭环（Dify 链1/Chatwoot 链4 对标）✅
- 文章状态机 draft/published/archived（hc_status列，双向同步 public_visible）
- 公开详情 views 自增原子计数 + Top 文章效果统计 API
- 检索测试 API（Retrieval Testing 对标）+ help_center_test_records 记录表 → **hits=1 record_id=2 实证**
- 修复：列名对齐 hc_status/hc_views

### D. AI 会话链闭环（Dify annotation-reply 对标）✅
- bad case 一键标注 API（POST /api/faqs/annotate）：相似度≥0.92 更新既有标注/否则新建，Layer1 命中直返标准答案
- 修复：FAQ AgentID 强约束兼容（标注归属共享池语义）

### E. 合规链补丁（Mautic 链3 对标）✅
- Postmark bounce(HardBounce)/spamcomplaint → 自动写入全局 DNC（source=email_hard_bounce），与既有 IsBlocked 前置检查闭环

## 二、过程中修复的工程问题
1. frontend_aliases.go 被脚本覆盖损坏 → git show HEAD 恢复 + 路由重放（449 doReg 去重校验）
2. hc_status 列名漂移（模型名≠SQL列名）→ 对齐
3. FAQ AgentID 强约束冲突 → annotation 语义兼容
4. 并行会话环境竞争：8204 被 TRAE 会话 go run 持有并周期性重启旧代码 → 关键验证一律先 `ps -p <pid> -o lstart=` 核对进程启动时间与二进制 mtime

## 三、已知限制（诚实记录）
- eval hit=0：eval 走 legacy Search 路径（hybridSearcher 装配限制），主检索链路 rag-config/query 独立验证命中
- feishu/dingtalk/crm 连接器自动拉取 = not_implemented 契约（凭据+连通测试已就绪）
- 表单/落地页构建器、Smart send time、归因 ROI、知识版本化管线：超出本轮，记录为演进项

## 四、回归
vitest 174/174、后端 build+vet 绿、全链验证均在最新代码进程上完成

## 已提交
commit 见 git log（feat(r53)）→ Gitee + GitHub 双远端
