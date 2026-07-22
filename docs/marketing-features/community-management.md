# 通用社群管理 (Community Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `community-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 通用社群管理 |
| 功能名称（英文） | Community Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | community |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（community_groups / community_members / community_messages）
- [x] 后端 Service 与 Controller
- [x] 群组 CRUD + 成员 CRUD + 消息发送
- [x] 群统计
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

除 WhatsApp/Telegram/企微外，提供通用社群管理能力，作为业务底层抽象。

### 2.2 解决思路

- 群组：抽象的群组模型（id/name/owner/members）
- 成员：抽象成员（user_id/group_id/role/joined_at）
- 消息：群发/单发（文本/图片）
- 统计：成员数/消息数/活跃度

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 群组名 |
| 输入 | description | text | 否 | 描述 |
| 输入 | members | []string | 否 | 初始成员 |
| 输出 | group_id | int64 | 是 | 群组ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/community/groups | 群组列表 |
| POST | /api/community/groups | 创建群组 |
| GET | /api/community/groups/:id | 详情 |
| PUT | /api/community/groups/:id | 更新 |
| DELETE | /api/community/groups/:id | 删除 |
| GET | /api/community/groups/:id/members | 成员列表 |
| POST | /api/community/groups/:id/members | 添加成员 |
| DELETE | /api/community/groups/:id/members/:userId | 移除成员 |
| POST | /api/community/groups/:id/messages | 发送消息 |
| GET | /api/community/groups/:id/messages | 消息列表 |
| GET | /api/community/groups/:id/stats | 群统计 |

### 3.3 安全与合规

- 仅群主/管理员可管理
- 操作审计
- 内容审核

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 成员查询 | < 200ms | ~80ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/community.go` | 接口 |
| Service | `internal/service/community_service.go` | 业务 |
| Repository | `internal/repository/community_repo.go` | 数据 |
| Model | `internal/model/community.go` | 模型 |
| Infra | Redis | 缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 鉴权 |
| customer-360 | 客户关联 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[商户] → 创建群组
   → 写 community_groups
   → 添加成员 → 写 community_members
   → 发送消息 → 写 community_messages
   → Redis 累加统计
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建群组
2. 添加成员
3. 发送消息
4. 查看统计

### 5.2 系统处理流程

1. 鉴权
2. 业务校验
3. 写库
4. 更新缓存

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 群主不存在 | 404001 | 提示不存在 |
| 成员已存在 | 409001 | 提示已存在 |

---

## 六、数据库设计

### 6.1 核心表 community_groups

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 群组名 |
| description | text | | 描述 |
| owner_id | bigint | FK | 群主 |
| member_count | int | 默认 0 | 成员数 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

### 6.2 核心表 community_members

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| group_id | bigint | FK | 群组 |
| user_id | bigint | FK | 用户 |
| role | varchar(16) | | owner/admin/member |
| joined_at | timestamp | 非空 | |

### 6.3 核心表 community_messages

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| group_id | bigint | FK | 群组 |
| sender_id | bigint | FK | 发送者 |
| msg_type | varchar(16) | | text/image/file |
| content | text | 非空 | 内容 |
| created_at | timestamp | 非空 | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建群组 | 完整参数 | group_id | ✅ |
| TC-002 | 添加成员 | user_id | member_id | ✅ |
| TC-003 | 发送消息 | group_id + content | message_id | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_MEMBERS_PER_GROUP | MAX_MEMBERS_PER_GROUP | 5000 |

---

## 九、参考资料

- [community-whatsapp.md](community-whatsapp.md)
- [community-wecom.md](community-wecom.md)
- [agent-telegram-automation.md](agent-telegram-automation.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
