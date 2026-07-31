# 系统配置 (System Config)

> **所属模块**: system
> **功能 slug**: `system-config`
> **文档定位**: 商户级系统配置管理,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 系统配置 |
| 功能名称(英文) | System Config |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端配置管理页
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 配置变更审批流(关键配置)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

每个商户需要根据自身业务调整系统配置(站点名称、Logo、域名、邮件签名、签名规则、API 限流等),系统需要支持运行时动态调整。

### 2.3 关键算法或模型

- **配置分组**: basic(基础)、email(邮件)、sms(短信)、security(安全)、integration(集成)
- **加密存储**: 敏感配置(AES-256-GCM)
- **变更通知**: Redis Pub/Sub 广播失效
- **配置版本**: 保留历史版本(7 天),支持回滚

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | key | string | 是 | 配置键 |
| 输入 | value | string | 是 | 配置值 |
| 输入 | group | string | 是 | 配置分组 |
| 输入 | is_encrypted | bool | 否 | 是否加密 |
| 输入 | description | string | 否 | 说明 |
| 输出 | value | string | 是 | 当前值(敏感值脱敏) |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/system/config | 获取所有配置 |
| GET | /api/system/config/:key | 单项配置 |
| PUT | /api/system/config/:key | 更新配置 |
| POST | /api/system/config/batch | 批量更新 |
| GET | /api/system/config/:key/history | 历史版本 |
| POST | /api/system/config/:key/rollback | 回滚版本 |

### 3.3 安全与合规

- 敏感配置 AES-256-GCM 加密
- 权限控制(管理员才能改)
- 变更审计日志
- 回滚能力

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 配置查询 | < 50ms |
| 配置更新 | < 200ms |
| 广播失效 | < 1s |
| 并发 | ≥ 100 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/system_config | |
| Service | internal/service/system_config | |
| Repository | internal/repository/system_config | |
| Model | internal/model/system_config | |
| Infra | internal/infra/redis | 配置缓存 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 所有业务模块 | 读取配置 |
| Redis | 配置缓存与广播 |

### 4.3 数据流向

```text
[读取] → [Redis 缓存] → [未命中查 DB] → [返回]
[更新] → [DB] → [Redis 失效] → [Pub/Sub 广播] → [其他节点清除本地缓存]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统管理 → 系统配置"
2. 选择分组
3. 修改配置项
4. 保存(部分项需二次确认)
5. 立即生效

### 5.2 系统处理流程

1. 接收更新请求
2. 校验权限
3. 加密敏感项
4. 写 DB
5. 失效 Redis 缓存
6. Pub/Sub 广播
7. 记录审计日志

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 配置不存在 | 404070 | 404 |
| 权限不足 | 403070 | 403 |
| 值格式错误 | 400130 | 400 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `system_configs` | 配置主表 |
| `system_config_histories` | 历史版本 |

```sql
CREATE TABLE system_configs (
  id BIGINT PRIMARY KEY,
  
  config_key VARCHAR(128) NOT NULL,
  config_group VARCHAR(32) NOT NULL,
  config_value TEXT,
  is_encrypted BOOLEAN DEFAULT false,
  value_type VARCHAR(16) DEFAULT 'string',  -- string/number/boolean/json
  description TEXT,
  updated_by BIGINT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE KEY uk_merchant_key ( config_key)
);

CREATE TABLE system_config_histories (
  id BIGINT PRIMARY KEY,
  config_id BIGINT NOT NULL,
  config_value TEXT,
  changed_by BIGINT,
  change_note TEXT,
  changed_at TIMESTAMP NOT NULL,
  INDEX idx_config (config_id, changed_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 查询配置 | key | 正确值 | 待执行 |
| TC-002 | 更新配置 | 合法值 | 200 OK | 待执行 |
| TC-003 | 加密配置 | 敏感项 | DB 中加密 | 待执行 |
| TC-004 | 缓存命中 | 重复查询 | < 50ms | 待执行 |
| TC-005 | 广播失效 | 多节点 | 其他节点清除 | 待执行 |
| TC-006 | 历史版本 | 多次修改 | 全部保留 | 待执行 |
| TC-007 | 回滚 | 选择历史 | 恢复 | 待执行 |
| TC-008 | 权限校验 | 普通用户 | 403 | 待执行 |
| TC-009 | 跨实例隔离 | 商户 A key | 商户 B 不可见 | 待执行 |
| TC-010 | 批量更新 | 10 项 | 全部成功 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 加密密钥 | CONFIG_AES_KEY | - | 32 字节 |
| 历史保留 | CONFIG_HISTORY_DAYS | 7 | |
| 缓存 TTL | CONFIG_CACHE_TTL | 1h | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- SENSITIVE_DATA_ENCRYPTION.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
