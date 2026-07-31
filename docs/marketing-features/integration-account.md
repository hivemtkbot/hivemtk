# 集成账号管理 (Integration Account)

> **所属模块**: integration
> **功能 slug**: `integration-account`
> **文档定位**: 第三方平台账号对接管理,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 集成账号管理 |
| 功能名称(英文) | Integration Account |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | integration |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端账号管理页
- [x] 客户/订单/产品同步
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] Shopify 集成

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

用户使用多个外部系统(CRM、ERP、电商),需要数据互通。集成账号管理通过统一接口对接第三方平台,实现客户、订单、产品等数据的双向同步。

### 2.3 关键算法或模型

- **Connector 接口**: `Auth/Connect/PullCustomers/PullOrders/PullProducts/PushCustomer/PushOrder`
- **OAuth 2.0**: 授权码模式,refresh_token 自动续期
- **增量同步**: cursor + last_sync_time
- **冲突解决**: last_write_wins(以第三方为准)/ 以本地为准
- **限流**: 各平台 API 限流(滑动窗口)

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | platform | string | 是 | wechat_shop/youzan/shopify |
| 输入 | name | string | 是 | 账号名称 |
| 输入 | credentials | object | 是 | 凭证 |
| 输出 | account_id | int64 | 是 | 账号 ID |
| 输出 | auth_url | string | 否 | OAuth 授权 URL |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/integration/accounts | 账号列表 |
| POST | /api/integration/accounts | 创建 |
| PUT | /api/integration/accounts/:id | 更新 |
| DELETE | /api/integration/accounts/:id | 删除 |
| POST | /api/integration/accounts/:id/connect | 发起连接(OAuth) |
| POST | /api/integration/accounts/:id/disconnect | 断开 |
| POST | /api/integration/accounts/:id/sync-customers | 同步客户 |
| POST | /api/integration/accounts/:id/sync-orders | 同步订单 |
| POST | /api/integration/accounts/:id/sync-products | 同步产品 |
| POST | /api/integration/accounts/:id/test | 测试连接 |

### 3.3 安全与合规

- 凭证加密存储
- OAuth token 加密
- 频率限制遵守各平台
- 同步操作审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 测试连接 | < 5s |
| 同步 1000 客户 | < 5min |
| 同步 1000 订单 | < 10min |
| 并发账号 | ≥ 10 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/integration | |
| Service | internal/service/integration | 账号管理 + 同步 |
| Engine | internal/service/integration/connectors | 各平台 connector |
| Repository | internal/repository/integration | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 客户/订单 | 同步目标 |
| 同步日志 | 同步结果 |
| 定时任务 | 自动同步 |

### 4.3 数据流向

```text
[OAuth 授权] → [获取 token] → [存储凭证]
                                    ↓
[定时同步] → [调用 connector] → [拉取数据] → [映射] → [写入本地库]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"第三方对接 → 集成账号"
2. 选择平台(微信小商店/有赞/Shopify)
3. OAuth 授权或填写 API Key
4. 测试连接
5. 启用同步策略(全量/增量/实时)
6. 查看同步日志

### 5.2 系统处理流程

1. 接收创建/连接请求
2. 跳转第三方 OAuth
3. 回调获取 token
4. 加密存储
5. 触发全量同步
6. 后续定时增量同步

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 授权失败 | 500170 | 重试授权 |
| token 过期 | 500171 | 自动 refresh |
| 限流 | 429030 | 退避重试 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `integration_accounts` | 集成账号 |
| `integration_sync_logs` | 同步日志 |

```sql
CREATE TABLE integration_accounts (
  id BIGINT PRIMARY KEY,
  
  platform VARCHAR(32) NOT NULL,  -- wechat_shop/youzan/shopify
  name VARCHAR(128) NOT NULL,
  credentials_encrypted TEXT,  -- 加密凭证
  access_token_encrypted TEXT,
  refresh_token_encrypted TEXT,
  token_expires_at TIMESTAMP,
  sync_customers BOOLEAN DEFAULT false,
  sync_orders BOOLEAN DEFAULT false,
  sync_products BOOLEAN DEFAULT false,
  sync_interval_minutes INT DEFAULT 60,
  last_sync_at TIMESTAMP,
  status VARCHAR(16) DEFAULT 'disconnected',  -- connected/disconnected/error
  error_message TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, platform, deleted_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建账号 | 完整参数 | 200 OK | 待执行 |
| TC-002 | OAuth 授权 | 跳转 | 回调成功 | 待执行 |
| TC-003 | API Key 模式 | 填写 key | 测试通过 | 待执行 |
| TC-004 | 凭证加密 | DB | 加密 | 待执行 |
| TC-005 | 测试连接 | 触发 | 成功 | 待执行 |
| TC-006 | 同步客户 | 触发 | 拉取并写入 | 待执行 |
| TC-007 | 同步订单 | 触发 | 拉取并写入 | 待执行 |
| TC-008 | 增量同步 | cursor | 仅拉新 | 待执行 |
| TC-009 | 定时同步 | cron | 自动执行 | 待执行 |
| TC-010 | token 过期 | 模拟 | 自动 refresh | 待执行 |
| TC-011 | refresh 失败 | 模拟 | 标记异常 | 待执行 |
| TC-012 | 限流 | 模拟 | 退避重试 | 待执行 |
| TC-013 | 断开连接 | 触发 | 清除 token | 待执行 |
| TC-014 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |
| TC-015 | 同步去重 | 重复客户 | 不重复 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 加密密钥 | INTEGRATION_AES_KEY | - | |
| 同步默认间隔 | INTEGRATION_SYNC_INTERVAL | 60min | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.12 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
