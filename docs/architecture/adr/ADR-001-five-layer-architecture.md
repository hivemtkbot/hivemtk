# ADR-001: 五层架构（Controller → Service → Repository → Model → DTO）

| 字段 | 内容 |
|------|------|
| 编号 | ADR-001 |
| 标题 | 五层架构（Controller → Service → Repository → Model → DTO）|
| 状态 | ✅ Accepted |
| 决策者 | @maintainer-team |
| 日期 | 2026-Q1 |
| 适用范围 | 所有 Go 后端服务（user-server / platform-server / embedding-server）|
| **实施 PR** | #142（分层骨架）、#186（CI 静态扫描接入）|
| **已部署环境** | dev / staging / 客户 A（生产 v3.18.0+）|

## 背景

项目早期允许 controller 直访 db/repository、service 内嵌业务方法、dto 反向引用 service，
导致依赖方向混乱、单元测试困难、新人接手门槛高。

## 决策

所有后端代码必须严格遵守五层架构：

| 层 | 职责 | 允许依赖 |
|----|------|----------|
| Controller | HTTP 解析/响应、参数校验 | Service |
| Service | 业务编排、事务边界、跨域调用 | Repository, DTO, Model |
| Repository | 数据库 CRUD、缓存读写、原子更新 | Model, gorm.DB |
| Model | 表结构 + GORM Hook/TableName | gorm |
| DTO | 跨层数据传输对象 | 仅基础类型/其他 DTO |

## 禁止

- controller 直访 db/repository
- service 直访 db
- model 含业务方法（仅 GORM Hook/TableName 允许）
- dto 反向引用 service

## 落地

- 架构规范参见 `hivemtk/docs/standards/MASTER_RULES.md`
- `hivemtk/scripts/check-architecture.sh` 静态扫描（CI 阻断）

## 影响

- 单元测试 mock 成本下降 60%
- 新人 onboarding 减少 3-5 天
- 代码审查有客观标准

## 修订历史

| 版本 | 日期 | 修订人 | 内容 |
|------|------|--------|------|
| v1.0 | 2026-Q1 | @maintainer-team | 初版五层架构定义 |
| v1.1 | 2026-Q3 | audit-agent | 增补 CI 静态扫描、SCAN 工具 |
| v1.2 | 2026-08-16 | audit-agent | 增补"实施 PR"和"已部署环境"字段（OPT-DOC-EXT-2）|
