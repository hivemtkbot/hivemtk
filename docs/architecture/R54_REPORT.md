# R54 业务正确性深测报告（2026-08-31 · 第十六轮：规则引擎断链根因修复）

> 方法：单测内联定位（TestDB 隔离+同步 Dispatch）→ 根因实锤 → 修复 → 单测+端到端双重实证

## 一、抓到的真业务缺陷（1 个，已修复+双重实证）

**message_inbound 规则链断裂**：
- 现象：配置"消息包含关键词→自动回复"规则后，普通进行中会话收到命中消息，自动回复**从未产生**（message_hub 0 行）
- 根因链（单测逐级定位）：
  1. `Dispatch` 缺 `inboundText` 参数 → `matchConditions` 的 content 字段恒为空串 → `contains` 永不命中
  2. 二次缺陷：controller 里仅 reopen 发生时才分发事件 → 非关闭会话根本进不了规则引擎（已在上轮修复中发现并修）
- 修复：`DispatchWithText(ctx, event, sessionID, inboundText, session)` 完整入口 + Push(inbound) 传 `m.Content` + 对所有 inbound 消息分发
- **单测实证**：TestR54RuleInboundChain PASS（run_count=1 outbound=1）
- **端到端实证**：真实推送命中消息 → message_hub 出现"R54自动回复内容"出站行

## 二、本轮通过的既有业务验证
- A1 closed→CSAT 自动触发（新会话全链：triggered_by=auto→submit→stats avg=4）+ **幂等防重**（reopen 后二次 close 不重复建单）
- 规则条件不匹配→run_count=0（不误触发）

## 三、测试工具教训（诚实记录）
- testutil.NewTestDB 是独立测试库——生产 psql 查不到 → 断言必须在测试进程内做
- Dispatch 是异步 goroutine → 断言前必须等待；诊断优先用同步调用
- 大量 shell 循环 cd 错目录浪费轮次——后续统一绝对路径

## 四、回归
vitest 174/174、vite build ✓、后端 build+vet 绿、测试残留清理（r54 测试文件已入正册保留）

## 已提交
commit 见 git log（fix(r54)）→ Gitee + GitHub 双远端
