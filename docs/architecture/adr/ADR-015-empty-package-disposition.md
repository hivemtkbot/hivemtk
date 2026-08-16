# ADR-015：空壳包逐项处置（P3-1）

| 字段 | 内容 |
|------|------|
| 编号 | ADR-015 |
| 标题 | 空壳包逐项处置（P3-1）|
| 状态 | ✅ Accepted（已执行）|
| 决策者 | @maintainer-team |
| 日期 | 2026-08-10 |
| 适用范围 | user-server 6 个疑似空壳包处置 |
| **实施 PR** | #297（plugin 删除 + 注释定性）|
| **已部署环境** | dev / staging / 客户 A（生产 v3.18.0+）|

## 背景

架构审计（A3）标记了 6 个疑似空壳/占位包：`reach`、`domain`、`contract`、
`identity`、`plugin`、`integration`（均位于 `user-server/internal/` 下）。
P3-1 要求按「补齐或删除」逐项决策并记录。

## 决策

| 包 | 现状核查 | 决策 | 理由 |
|---|---|---|---|
| `internal/reach` | 仅 `card/template` 子包（8 文件），被 `service/*_card.go`×4 + `controller/live_code.go` 引用 | **保留** | 有真实内容与消费方，是触达域卡片模板子模块；`service/reach_*` 的物理拆分随 P2-1 大域拆分滚动执行，届时本包升级为触达域落点 |
| `internal/domain` | `asset`（validator/entity）+ `errors`（codes），被 repository/controller/service 共 5 处引用 | **保留** | 资产域内核与统一错误码，非空壳 |
| `internal/contract` | `stream_engine.go`（StreamEngineInterface），被 `controller/chat_ws.go` 引用 | **保留** | 依赖倒置契约包（L0 横切层），用于打破 controller↔service 循环依赖，定位正确 |
| `internal/identity` | `normalize.go` + 测试，被 `controller/customer_oneid.go` + `service/customer_identity*.go`×4 引用 | **保留** | OneID 身份规范化纯函数包，定位正确 |
| `internal/plugin` | `main/main.go`（空壳导出）+ `auth_plugin.so`（12MB 构建产物）；全仓零引用、无 `plugin.Open` 调用、无构建脚本入口；且整体被 .gitignore 忽略（未入库） | **删除** | 死代码 + 本地产物残留，已于本地移除 |
| `internal/integration` | 根目录仅 `integration_test.go`（7 个跨域集成测试，`go test` 全绿）；`templates` 子包被 `service/integration_template.go` 引用 | **保留并定性** | 根目录定性为「跨域集成测试落点」（test-only 包，不放生产代码）；`templates` 为 ERP/CRM 预置模板数据包 |

## 后果

- `internal/plugin` 从工作区移除（本就未入库，不影响 git 历史）。
- 其余 5 包全部有真实消费方，维持现状；后续域拆分（P2）时 `internal/reach`
  作为触达域物理拆分的落点，`internal/domain` 随资产域演进扩展。
- 新增约束：`internal/` 下禁止再出现无生产代码、无引用的占位目录；
  test-only 包必须在包注释中显式定性。

## 修订历史

| 版本 | 日期 | 修订人 | 内容 |
|------|------|--------|------|
| v1.0 | 2026-08-10 | @maintainer-team | 初版空壳包处置决策 |
| v1.1 | 2026-08-16 | audit-agent | 增补"实施 PR"和"已部署环境"字段 |
