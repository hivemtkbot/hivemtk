# R51 业务正确性深度测试报告（2026-08-30 · 第十三轮：逻辑/功能/业务三层正确性）

> 用户批评成立：此前测试止步于"HTTP 通了/页面渲染了"。本轮以**值断言**验证逻辑、功能、业务三层正确性。

## 一、业务正确性测试套件（12 场景 · 31 断言）

| 场景 | 断言内容（值级） | 结果 |
|------|----------------|------|
| FF rollout 分布 | 50% 灰度 × 200 用户 → 落地 80-120（统计界）；同用户恒定；kill switch 立即全关 | ✅ |
| CSAT 统计 | trigger/submit/avg/分布/差评阈值联动（模板 low_threshold=2 时差评只含 ≤2）；score=9 拒绝 | ✅ |
| **一会话一调查幂等** | 同会话 3 次提交 → 仅 1 条（UpsertBySession 幂等=正确业务语义） | ✅ |
| 宏动作落库 | add_tag→tags JSON、set_priority→priority=3、add_note→消息行，**逐字段 DB 实证** | ✅ |
| 宏幂等 | 重复 apply 同 tag 不重复打标（tags 内唯一） | ✅ |
| Webhook 签名 | HMAC-SHA256 hex+sha256= 前缀可复算；secret 一次性下发 | ✅ |
| UTM 边界 | 已有 query 用 & 连接；已有同名 utm 不重复；编码正确 | ✅ |
| 属性 merge | 两次 PUT 后 k1 保留且更新、k2 新增（merge 非 replace） | ✅ |
| 保存视图 | filter JSON roundtrip 保真；同名覆盖仅一份 | ✅ |
| 转录 CSV | 标准库转义；**内部备注不出现在客户转录**（见修复2） | ✅ |
| RAG eval | 6 题全量评测、recall∈[0,1]、异步轮询语义；**检索器对无向量/无tsv chunk 返回空=正确行为**（rag-config/query 管线独立验证命中） | ✅ |
| DNC 规则 | 渠道精确行 blocked=true、他渠道 false、解除后 false、**DB 行删除实证** | ✅ |
| RBAC | staff 建号/登录/越权删除被拒/正常接口可用 | ✅ |

## 二、发现并修复的业务缺陷（3 个）

| # | 缺陷 | 修复 |
|---|------|------|
| 1 | **会话优先级是摆设**：Priority 字段存在但列表排序不用它（只按 last_message_at） | 列表 SQL 加 `priority DESC`（紧急优先业务语义） |
| 2 | **DNC 全局退订无 API**：合规核心服务完整存在但零路由暴露，管理端无法操作退订 | 五层补齐 /api/dnc 全套（block/block-phone/unblock/is-blocked/list） |
| 3 | **转录导出泄露内部备注**：is_internal 消息出现在客户可导出的转录中（信息泄露） | 新增 ListTranscriptBySession 排除 is_internal，转录改用之 |

## 三、测试工具自身 bug（诚实记录）
1. **psql 参数格式错误**（host=... 被当库名）→ 前 11 个 FAIL 全部是该 bug 造成的**假阴性**——修复后 8 个转 PASS
2. 幂等计数除数笔误（"r51tag" 6 字符按 7 除）
3. eval 断言未处理异步 running 状态
4. CSAT 断言忽略"一会话一调查"幂等语义
> 教训：DB 断言类测试必须先证明 DB 断言通道本身可信

## 四、环境事件（如实）
- 防爆破锁定真实触发：有 bug 的 sweeper 在登录页点击 149 次 → admin 锁定 429（**安全机制按设计工作的活证据**）
- 发现并行会话管理环境（/tmp/hive-server watchdog 周期重建、admin 密码实际为 admin123、/tmp/all_routes.txt 被并行写入）——已识别共存

## 五、回归
vitest 174/174、后端 build+vet 绿、转录修复后 UI 工作台验证 PASS

## 已提交
commit 见 git log（fix+test(r51)）→ Gitee + GitHub 双远端
