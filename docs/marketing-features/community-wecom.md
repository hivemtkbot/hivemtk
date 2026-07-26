# 企业微信集成 (WeCom Integration)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `community-wecom`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 企业微信集成 |
| 功能名称（英文） | WeCom Integration |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | community |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（wecom_accounts / customers / groups / members / messages / tags）
- [x] 后端 Service 与 Controller
- [x] 前端页面（账号/客户/群组/群成员/消息/标签 6 模块）
- [x] 企微 API 集成（客户管理/群管理/消息推送）
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

企业微信作为企业内部协作 + 客户管理工具。提供账号接入、客户管理、群管理、消息推送、标签能力。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | corp_id | string | 是 | 企业 ID |
| 输入 | corp_secret | string | 是 | 应用 Secret |
| 输入 | agent_id | int | 是 | 应用 ID |
| 输出 | account_id | int64 | 是 | 账号ID |
| 输出 | access_token | string | 是 | 访问 Token |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/wecom/accounts | 账号列表 |
| POST | /api/wecom/accounts | 添加账号 |
| GET | /api/wecom/customers | 客户列表 |
| GET | /api/wecom/groups | 群组列表 |
| GET | /api/wecom/groups/:id/members | 群成员 |
| POST | /api/wecom/messages/send | 发送消息 |
| GET | /api/wecom/tags | 标签 |
| POST | /api/wecom/tags | 创建标签 |
| POST | /api/wecom/customers/:id/tags | 打标签 |

### 3.3 安全与合规

- 凭据加密
- access_token Redis 缓存
- 消息内容审计
- 频率限制

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 消息推送 | < 1s | ~300ms |
| 客户同步 | 100/批 | 100/批 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/wecom.go` | 接口 |
| Service | `internal/service/wecom_service.go` | 业务 |
| Repository | `internal/repository/wecom_repo.go` | 数据 |
| Model | `internal/model/wecom.go` | 模型 |
| Infra | `internal/wecom/api.go` + Redis | 企微 API 客户端 + 缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 鉴权 |
| obs-config | 头像存储 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |
| cs-session | 客服会话 |

### 4.4 数据流向

```text
[商户] → 添加账号
   → 调企微 API 获取 access_token
   → Redis 缓存（2h TTL）
   → 写 wecom_accounts
   → 同步客户/群组/标签
   → 调企微 API 拉数据
   → 写 wecom_customers / groups / tags
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加企微账号
2. 同步客户/群组
3. 查看客户/群组列表
4. 发送消息/打标签
5. 查看消息记录

### 5.2 系统处理流程

1. 鉴权
2. access_token 缓存复用
3. 调企微 API
4. 数据转换
5. 写库

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| Token 过期 | 401001 | 自动刷新 |
| 客户不存在 | 404001 | 提示不存在 |
| 频次超限 | 429001 | 限流等待 |

---

## 六、数据库设计

### 6.1 核心表 wecom_accounts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| corp_id | varchar(64) | 非空 | 企业 ID |
| agent_id | int | 非空 | 应用 ID |
| corp_secret_enc | varchar(512) | 非空 | 加密 Secret |
| name | varchar(128) | | 应用名 |
| status | tinyint | 非空 | 0=离线 1=在线 2=禁用 |

### 6.2 核心表 wecom_customers

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| external_userid | varchar(128) | UNIQUE | 外部用户 ID |
| name | varchar(64) | | 姓名 |
| avatar | varchar(512) | | 头像 |
| type | tinyint | | 1=微信用户 2=企业用户 |
| gender | tinyint | | 性别 |
| unionid | varchar(128) | | UnionID |
| follow_userid | varchar(64) | | 跟进员工 ID |

### 6.3 核心表 wecom_groups

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| chat_id | varchar(64) | UNIQUE | 群 ID |
| name | varchar(128) | | 群名 |
| owner | varchar(64) | | 群主 |
| member_count | int | | 群成员数 |

### 6.4 核心表 wecom_messages

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| touser | varchar(128) | | 接收者 |
| toparty | varchar(64) | | 接收部门 |
| totag | varchar(64) | | 接收标签 |
| msgtype | varchar(16) | | text/image/news |
| content | text | | 消息内容 |
| status | varchar(16) | | success/failed |
| error_msg | text | | 错误信息 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 添加账号 | corp_id + secret | account_id | ✅ |
| TC-002 | 同步客户 | 同步请求 | 客户列表 | ✅ |
| TC-003 | 发送消息 | touser + content | 200 | ✅ |
| TC-004 | Token 缓存 | 重复请求 | 命中缓存 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| WECOM_API_BASE | WECOM_API_BASE | https://qyapi.weixin.qq.com |
| WECOM_TOKEN_TTL | WECOM_TOKEN_TTL | 7200 |

---

## 九、参考资料

- [community-management.md](community-management.md)
- [cs-session.md](cs-session.md)
