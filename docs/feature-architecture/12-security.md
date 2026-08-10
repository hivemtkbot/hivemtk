# 十二、安全与权限域（2 功能）

> 架构约束：严禁无开关默认开启的行级权限。本域为显式配置项，符合铁律。

---

## 12.1 权限系统（permission-system，角色/菜单/按钮级）
| 端点 | 方法 | 关键入参 | 论证 |
|------|------|----------|------|
| /api/permissions/roles | CRUD | `name`、`menus[]`、`buttons[]` | 角色-菜单-按钮三级；超管角色受保护（不可删/降权，见 1.2 锚点）。 |
| /api/permissions/check | POST | `user_id`、`resource`、`action` | 鉴权集中在中间件，handler 不再各自判断；返回 bool + 原因。 |

## 12.2 行级数据权限（row-level-security，data_scope 中间件）
| 端点 | 方法 | 关键入参 | 论证 |
|------|------|----------|------|
| /api/data-scope/config | PUT | `role_id`、`scope`(all/dept/self) | `scope` 显式配置（默认 self，非全量开放）；中间件按 scope 注入查询条件。 |
| /api/data-scope/apply | GET | `resource` | 应用 data_scope 到查询；与 customer-360（8.2）联动。 |

---

## 头脑风暴与优化论证（全域）
- **问题**：data_scope 中间件需每个查询手动拼接条件，漏拼即越权。
- **优化**：data_scope 改为 GORM `plugin/scopes` 自动注入（所有经 repository 的查询默认带 scope），handler 无法绕过；scope 变更影响面审计（11.5）。
- **论证**：自动注入根除「漏拼」类越权；显式默认 self 符合铁律。
- **风险**：自动注入需确保跨表关联查询 scope 正确传播（JOIN 场景易漏），需集成测试覆盖。
