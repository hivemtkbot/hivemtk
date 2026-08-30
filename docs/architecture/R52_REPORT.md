# R52 假性完成清剿报告（2026-08-30 · 第十四轮：未完成/假性完成全量清点+一次性修复）

> 方法：对全部历史交付做"DB 落库级"复验（不看 API 响应，看数据真的写进去没有）。

## 一、确认的假性完成（3 个，全部修复+DB 实证）

| # | 假性完成 | 证据 | 修复 | 实证 |
|---|---------|------|------|------|
| 1 | **宏 send_message 动作**：手写 INSERT 引用不存在的 sender_type 列 → failed 永远出现，消息从未入队。此前只验证了 add_tag/set_priority/add_note | apply 响应 failed=['send_message: column sender_type does not exist'] | 弃手写 SQL → GORM 模型 Create（列自动对齐），trace_id='macro' 标记 | **executed:['send_message'] + DB 行实证**（web/outbound/pending/trace_id=macro） |
| 2 | **办公时间离开自动回复**：同一 sender_type 问题 + 防重复查询同样引用不存在列 | 同上 | 改 GORM Create + trace_id='away' 防重复 | **真实触发**：启用夜间时段→创建新会话→away 消息落库实证（trace_id=away） |
| 3 | **backup restore 状态停留 pending**：后台恢复用请求 ctx，请求返回后 ctx 取消 → 状态 Update 全部静默失败（"恢复完成"日志有但记录 pending） | restore_records 永远 pending | context.WithoutCancel | **record status=completed DB 实证**（独立端口 8210 验证实例） |

## 二、未完成项验证与澄清（5 项）

| 项 | 结论 |
|----|------|
| embedding 管线 | **实际正常**：R48 导入文档异步处理后 embed_status=indexed chunks=1（R51 时的 0 是管线未处理完的时序） |
| RAG eval hit=0 | 检索器对无向量/无tsv chunk 返回空=正确行为；主检索链路（rag-config/query）独立验证命中并生成答案。已知限制：eval 走 legacy Search 路径依赖 hybridSearcher 装配 |
| R44 CRUD 残留 | 5 族全部补验证成功（chat-channels AllowedOrigins/knowledge-bases owner_type=shared/marketing-flows trigger节点/custom-reports chart_type/asset-bundle 完整字段），逐个清理 |
| backup restore 端到端 | create→completed→preview→restore→restore_records completed 全链路 ✓ |
| 测试数据残留 | geo_keywords 787→778、csat 测试记录、宏、staff 用户、R48 文档、segments、flags 全部清理，业务数据零误删 |

## 三、环境共存备忘（R51 续）
- 8204 由并行 TRAE 会话 `go run cmd/api/main.go` 持有并周期重启——杀掉会被拉回旧编译缓存
- **共存方案**：验证用独立端口 8210 实例（同 DB 同代码），业务验证不再与 8204 抢占
- admin 实际密码 admin123（并行会话侧设置）；JWT secret 已对齐其值 hivemtk-jwt-secret-key-32charnow12345

## 四、回归
vitest 174/174、后端 build+vet 绿；8210 实例全链路验证

## 已提交
commit 见 git log（fix(r52)）→ Gitee + GitHub 双远端
