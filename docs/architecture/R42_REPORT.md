# R42 闭环报告（2026-08-29 · 第四轮循环：一次性调研论证→开发→测试）

> 采纳标准（用户钦定）：**站在用户角度，功能是否需要，是否有必要 — 其他不考虑**

## 本轮焦点论证
R40 建立的连接器凭据 API 属"后端就绪前端未接线"（ZOMBIE②类原则：本次接线而非搁置）。
调研确认 Notion 官方 API（/v1/search + /v1/blocks/{id}/children，真实连通验证）足以支撑一键拉取导入；
飞书/钉钉/CRM 需更复杂 OAuth/导出协议 → 诚实返回明确 not_implemented 契约（不假装可用）。

## Step5 开发（4 项）

| # | 交付 | 明细 |
|---|------|------|
| K1 | Notion 一键拉取 | kb_connector_pull.go：search 列页→blocks 提取纯文本→走既有 KB Import 管线（metadata.connector=notion, source_ref=url）；串行限速尊重 3rps；max_pages 默认10上限20；逐页结果（imported/failed/skipped）；标题从首个 title 类属性提取 |
| K2 | pull 端点 | POST /api/knowledge/connectors/:source/pull {product_id, query, max_pages}（单结构体合并绑定修复 Gin body 单读） |
| K3 | 连接器管理页 | Connectors.vue：4源卡片/凭据表单（脱敏回显 show-password）/测试连接/选择目标KB/一键拉取/结果表格；路由 knowledge/connectors 挂载 |
| K4 | 脏数据清理 | 5平台卡片表 img.example.com 占位外链 SQL 清理（douyin_cards 1 行置空） |

## Step6 测试
- 构建：后端 build+vet 绿；vite build ✓；vitest 174/174；service 回归绿
- 契约验证：
  - notion pull（假token）→ "HTTP 401: API token is invalid"（真实上游 API 调用链路通）
  - feishu pull → 明确"自动拉取尚未实现（凭据与连通测试已就绪；内容接入请用外部导入推送或 OpenAPI 集成）"
  - dingtalk pull（未配置）→ 同上明确契约
- UI：/knowledge/connectors 页 API 200、标题与 Notion 卡片渲染、零 console 错误
- 修复 1 个开发中发现的 bug：Gin ShouldBindJSON 单读限制 → 合并结构体绑定

## 已提交
commit 见 git log（feat(r42)）→ Gitee + GitHub 双远端

## 遗留
- feishu/dingtalk/crm 自动拉取（凭据+测试已就绪，导出协议属后续迭代）
- Connectors.vue 保存凭据时前端回显的是脱敏值——用户需重新输入完整密钥才能更新（读侧脱敏的标准权衡，已在表单占位符提示）
