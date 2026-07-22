# 方向E 实施计划：repository 层统一 context 透传

## 一、预调研结果（2026-07-22）

### 1. 总体规模
- 总方法数：**1057**
- 缺 ctx 方法数：**843**（占比 79.8%）
- 涉及文件：**90 个**（73 个在 internal/repository，5 个在 internal/content/repository，17 个在 internal/aiagent/agent/tooluse，10 个在 internal/aiagent/agent）

> 注：脚本按 `(r *xxx)` 接收者识别，部分工具层 struct 不带 `*xxx` 接收者未计入（如 StreamStateMachine 中的方法可能为非指针接收者）。本表以"含方法签名的文件"为统计单位。

### 2. 按包分组

| 包 | 文件数 | 缺 ctx 方法数 | 总方法数 | 占比 |
|---|---|---|---|---|
| internal/repository | 73 | 725 | 863 | 84.0% |
| internal/content/repository | 5 | 60 | 60 | 100.0% |
| internal/aiagent/agent/tooluse | 17 | 118 | 194 | 60.8% |
| **合计** | **90** | **843** | **1057** | **79.8%** |

### 3. 内部 repository Top 20（按缺失数降序）

| 排名 | 文件 | 缺 ctx | 总数 |
|---|---|---|---|
| 1 | customer_session.go | 42 | 42 |
| 2 | integration.go | 40 | 40 |
| 3 | wecom.go | 33 | 33 |
| 4 | feishu.go | 27 | 27 |
| 5 | auto_reply.go | 27 | 27 |
| 6 | ai_agent_repository.go | 27 | 27 |
| 7 | sms.go | 25 | 25 |
| 8 | unified_message.go | 21 | 21 |
| 9 | domain_pool.go | 20 | 20 |
| 10 | whatsapp.go | 20 | 20 |
| 11 | generic.go | 19 | 19 |
| 12 | customer_rfm.go | 16 | 16 |
| 13 | marketing_flow.go（content）| 16 | 16 |
| 14 | rfm_rule.go | 15 | 15 |
| 15 | ai_content.go（content）| 15 | 15 |
| 16 | script_template.go（content）| 15 | 15 |
| 17 | upgrade.go | 14 | 14 |
| 18 | community.go | 13 | 13 |
| 19 | sms_tracking.go | 13 | 13 |
| 20 | order.go | 12 | 12 |
| 20 | backup.go | 12 | 12 |
| 20 | message_hub_inbox.go | 12 | 12 |
| 20 | tiktok_card.go | 12 | 12 |

### 4. tooluse 包 Top 10

| 排名 | 文件 | 缺 ctx | 总数 |
|---|---|---|---|
| 1 | stream_state_machine.go | 14 | 15 |
| 2 | private_message_tools.go | 13 | 16 |
| 3 | circuit_breaker.go | 12 | 12 |
| 4 | registry.go | 12 | 12 |
| 5 | result_cache.go | 11 | 11 |
| 6 | dead_letter.go | 9 | 11 |
| 7 | executor.go | 9 | 15 |
| 8 | db_audit_persister.go | 9 | 11 |
| 9 | tool_router.go | 8 | 9 |
| 10 | double_intercept.go | 7 | 10 |
| 10 | decorator.go | 7 | 10 |

## 二、实施批次（按依赖顺序）

### 批次 1：基础独立模块（无外部依赖，可独立 go build）
- system_config (2) ✅ 简单，可立即全量 ctx 化
- backup (12) ✅ 独立
- obs_config (11) ✅ 独立
- payment_config (4) ✅ 独立
- security_audit (4) ✅ 独立
- generic (19) ✅ 通用基类
- user_blacklist (4) ✅ 已部分含 ctx，复检
- short_link (8) + short_link_access (9) ✅ 配对

### 批次 2：用户/客户域（核心业务）
- user (11), account (7)
- customer_repository (11), customer_tag (5), customer_event (5)
- customer_rfm (16), rfm_rule (15)
- clue (11), clue_score (9), order (12)
- message (4), message_hub_inbox (12), unified_message (21)
- user_tag (9)

### 批次 3：customer_session（最大头）
- customer_session (42) ← 已有 portcontract 模式
- 适配：tooluse 工具层 + email service

### 批次 4：集成/平台
- integration (40), integration_template (8)
- wecom (33), feishu (27), whatsapp (20), whatsapp_template (5)
- auto_reply (27), ai_agent_repository (27)
- community (13), sales_persona (8), upgrade (14)

### 批次 5：卡券/短链/抖音
- tiktok_card (12), douyin_card (8), xiaohongshu_card (8), kuaishou_card (10), xianyu_card
- live_code (7), live_code_qr (8), card_access_repository (9)
- domain_pool (20)

### 批次 6：邮件/短信
- email_draft (5), email_jobs (5), email_list (9), email_send (6), email_smtp (5), email_tracking (10), email_unsubscribe (7)
- sms (25), sms_tracking (13), sms_unsubscribe (7), sm_list (6)

### 批次 7：content repository
- marketing_flow (16), ai_content (15), script_template (15)
- material_category (7), material (7)

### 批次 8：aiagent tooluse
- tool (4), registry (12), executor (9/15)
- circuit_breaker (12), result_cache (11), dead_letter (9/11)
- db_audit_persister (9/11), tool_router (8/9), double_intercept (7/10), decorator (7/10)
- stream_state_machine (14/15), private_message_tools (13/16), reach_integration_adapter (2/18)

## 三、每批次实施流程

1. **Repository 改造**：
   - 添加 `context` import
   - 给所有业务方法加 `ctx context.Context` 作为第一参数
   - 在 `r.db.WithContext(ctx)` 处替换为 ctx-aware 调用
   - 保留 GetDB / WithTx / SetDB / DB 不变
2. **Service 适配**：
   - 给所有 service 方法加 ctx
   - 替换 repository 调用为 `s.repo.Xxx(ctx, ...)`
3. **Controller 适配**：
   - 通过 `c.Request.Context()` 获取 ctx 传入 service
4. **测试修复**：
   - 测试调用追加 `context.Background()`
5. **验证**：
   - `go build ./...` 通过
   - `go vet ./...` 无 error
   - `bash scripts/check-architecture.sh` 错误数减少
   - 重新跑 `check_ctx_missing.go` 数字下降

## 四、验证指标

- 初始缺 ctx 方法数：843
- 完成批次 1-2 后目标：< 600
- 完成批次 1-4 后目标：< 400
- 完成批次 1-6 后目标：< 200
- 全部完成后目标：**0**

## 五、约束

- 不破坏现有五层架构
- 不引入新的 import cycle
- 保留所有 GetDB / WithTx / DB / SetDB 基础方法
- 每次提交独立可编译（不出现中间态全量 build 失败）
- 重大变更需走 git commit
