# R46 争议问题仔细检查报告（2026-08-29 · 第八轮：假实现全部清算）

> 用户指令：发现的问题要修复，有争议的问题要仔细检查。
> 方法：对我此前"标签化带过"的每个裁决重新取证，假实现全部修真。

## 一、发现并修复的假实现（5 个，全部为"看着能用实际骗人"级别）

| # | 假实现 | 证据 | 真修复 | 验证 |
|---|--------|------|--------|------|
| 1 | RfmMatrix `saveSegment()` 只弹"已保存"不落库；`viewDetail` 指向不存在路由 `/customerSegment/:id`；`reachSegment` 指向不存在的 `/reachPipeline/new`；`exportToReach` 只弹 info | 源码审查 | saveSegment→真实 POST /api/user-segments（新五层 CustomerSegment 表落库）；三个跳转全部改真实路由 | 构建+UI PASS |
| 2 | `POST /api/user-segments` 未注册（Builder.vue 保存分群 405） | 路由表 | 新增 CustomerSegment 模型+仓储（含白名单校验的规模估算 SQL）+服务+控制器+POST/GET 路由 | 构建通过 |
| 3 | DLQ batch-retry 空转：`UPDATE WHERE status='dead_letter'` 但 message_hub **无 dead_letter 状态**（实测永远 requeued=0）；GET /dlq、单条 retry、drop 整组缺失 | 状态枚举实测（delivered/failed/inflight/pending） | 死信语义修正为 status='failed'；四端点真实现：列表（含 failedAt/retries 从 extra JSON 提取）/单条重试(failed→pending)/批量重试(上限500防风暴)/丢弃 | 造 2 条 failed→列表 2 行→单条重试→剩 1 全闭环 ✓ |
| 4 | clue apply-suggestions 只计数不落库（注释"由管线承接"）；merge/force-create 端点不存在 | 组件 ClueImportResultDialog 契约（duplicates/existingClueId/row） | 三端点真实落库：apply-suggestions 按 action 逐条合并非空白名单字段；merge 单条合并；force-create 单条创建（schema 对齐 model.Clue：city/address/desc） | 构建通过 |
| 5 | RagEval run 同步执行（200 条×检索必 HTTP 超时，前端却提示"3-5 分钟完成"） | 前后端契约矛盾 | RunAsync：先落 running 记录(total=-1)→后台 goroutine 计算(10min 超时)→回填；失败标 total=-2 | 构建通过 |

## 二、有争议裁决的仔细复核（4 个）

| 争议点 | 取证 | 裁决 |
|--------|------|------|
| RFM 矩阵 R5F5 映射 | segment 实际枚举=5 个英文分层（champion/loyal/at_risk/potential/churn，GetLayerDescription 佐证），非 R×F 网格 | **我原映射错误**→修正为层间语义位置映射（champion→(5,5)/loyal→(4,4)/potential→(3,2)/at_risk→(2,3)/churn→(1,1)）；实测 14 客户落 potential→(3,2) ✓ |
| cohort 留存全 0 是否缺陷 | customers 软删后活跃 12 人全落 08/16 分群（size=12 正确）；customer_events 仅 2 条测试事件（v_r39 不在客户表） | **数据真实非缺陷**：无真实行为事件时留存=0 是诚实口径 |
| /api/platform/* 502 是否该优雅化 | 全仓扫描：前端 0 处调用（仅 Notifications 注释提及且已本地回退；消费方是 platform-web） | **维持 502**（操作型代理端点对无平台端环境报错语义正确） |
| TG bot 111/222/333 | bot_token=SECTEST_ 前缀（非真实 TG token 格式）+名称 sec_idor_*（IDOR 安全测试命名）+全仓无 seed 来源 | **安全测试残留数据**→停用（status=0，保留可追溯）消除重启外呼噪音 |

## 三、顺带修复
- email domain-reputation 表名错误（smtp_configs→email_smtp，字段 host→server）
- /user-segments/rfm/stats 重复注册冲突（doReg recover 吞掉后者——删除旧注册，新实现生效）
- message_hub 无 claim_count 列（retries 改从 extra JSON 提取 retry_count）

## 四、回归
- 后端 build+vet 绿；vitest 174/174；vite build ✓
- 6 个受影响页面 UI 遍历全 PASS（rfm-matrix/messageHub/connectors/rag-eval/deliverability/backup）
- DLQ 真实数据闭环：造 failed→列表→单条重试→剩余计数 ✓

## 已提交
commit 见 git log（fix(r46)）→ Gitee + GitHub 双远端
