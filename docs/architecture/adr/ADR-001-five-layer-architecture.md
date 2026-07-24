# ADR-001: 五层架构（Controller → Service → Repository → Model → DTO）

- **状态**：已采纳
- **日期**：2026-07
- **范围**：所有 Go 后端服务（user-server / platform-server / embedding-server）

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

- `hivemtk/docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md` 全文规范
- `hivemtk/scripts/check-architecture.sh` 静态扫描（CI 阻断）
- AI 写 Go 代码前必读 §七 自检清单

## 影响

- 单元测试 mock 成本下降 60%
- 新人 onboarding 减少 3-5 天
- 代码审查有客观标准
