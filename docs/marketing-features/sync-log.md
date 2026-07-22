# 同步日志 (Sync Log)

> **所属模块**: integration
> **功能 slug**: `sync-log`
> **文档定位**: 第三方平台数据同步日志与外部数据查询,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 同步日志 |
| 功能名称(英文) | Sync Log |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | integration |
| 优先级 | P1 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端日志查看页
- [x] 外部数据查询(客户/订单/产品)
- [x] 失败重试
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 同步监控大屏

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

数据同步可能出现部分失败(网络、限流、数据格式),需要详细日志便于排查;同时支持查询第三方平台的原始数据(客户/订单/产品)以做对比。

### 2.2 解决思路

每次同步记录详细日志(开始时间、结束时间、结果、新增/更新/失败数量、错误明细)。外部数据通过 connector 实时查询(不下沉到本地)。

### 2.3 关键算法或模型

- **日志级别**: success(全成功)/ partial(部分失败)/ failed(全失败)
- **失败重试**: 单条失败可重试
- **外部查询**: 透传请求到第三方 connector
- **日志保留**: 90 天(可配置)

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_id | int64 | 是 | 集成账号 ID |
| 输入 | sync_type | string | 是 | customers/orders/products |
| 输入 | page | int | 否 | 页码 |
| 输入 | page_size | int | 否 | 页面大小 |
| 输出 | logs | array | 是 | 日志列表 |
| 输出 | external_data | array | 是 | 外部数据 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/integration/sync-logs | 同步日志列表 |
| GET | /api/integration/sync-logs/:id | 日志详情 |
| POST | /api/integration/sync-logs/:id/retry | 重试 |
| GET | /api/integration/external-customers | 外部客户 |
| GET | /api/integration/external-orders | 外部订单 |
| GET | /api/integration/external-products | 外部产品 |
| GET | /api/integration/sync-stats | 同步统计 |

### 3.3 安全与合规

- 外部数据仅授权人员可见
- 日志脱敏(隐藏客户手机号中间四位)
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 日志查询 | < 300ms |
| 外部查询 | < 3s |
| 列表分页 | < 500ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/integration_sync_log | |
| Service | internal/service/integration_sync_log | |
| Repository | internal/repository/integration_sync_log | |
| Model | internal/model/integration_sync_log | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 集成账号 | 同步源 |
| Connectors | 外部查询 |

### 4.3 数据流向

```text
[同步任务] → [记录日志] → [用户查询]
[外部查询] → [connector] → [第三方 API] → [返回]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"第三方对接 → 同步日志"
2. 查看同步日志列表(时间/账号/类型/结果)
3. 点击详情查看具体错误
4. 对失败日志点击"重试"
5. 切换到"外部数据" Tab 查询原始数据

### 5.2 系统处理流程

1. 同步任务执行时,实时写入日志
2. 失败时记录具体错误堆栈
3. 用户查询时返回日志
4. 重试时重新调用 connector

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 日志不存在 | 404080 | 404 |
| 重试中 | 409010 | 提示 |
| 外部查询失败 | 500180 | 透传错误 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `sync_logs` | 同步日志主表 |
| `sync_log_items` | 同步明细(每条记录) |

```sql
CREATE TABLE sync_logs (
  id BIGINT PRIMARY KEY,
  
  account_id BIGINT NOT NULL,
  sync_type VARCHAR(32) NOT NULL,  -- customers/orders/products
  status VARCHAR(16) NOT NULL,  -- success/partial/failed
  total_count INT DEFAULT 0,
  success_count INT DEFAULT 0,
  failed_count INT DEFAULT 0,
  new_count INT DEFAULT 0,
  updated_count INT DEFAULT 0,
  duration_ms INT,
  error_message TEXT,
  triggered_by VARCHAR(16),  -- manual/cron/auto
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP,
  INDEX idx_data, account_id, started_at)
);

CREATE TABLE sync_log_items (
  id BIGINT PRIMARY KEY,
  sync_log_id BIGINT NOT NULL,
  external_id VARCHAR(128),
  action VARCHAR(16),  -- create/update/skip/failed
  error_message TEXT,
  raw_data JSONB,
  processed_at TIMESTAMP NOT NULL,
  INDEX idx_log (sync_log_id)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 日志列表 | 查询 | 正确分页 | 待执行 |
| TC-002 | 日志详情 | ID | 明细 | 待执行 |
| TC-003 | 失败重试 | 失败日志 | 重新执行 | 待执行 |
| TC-004 | 外部客户查询 | 账号 ID | 返回列表 | 待执行 |
| TC-005 | 外部订单查询 | 账号 ID | 返回列表 | 待执行 |
| TC-006 | 外部产品查询 | 账号 ID | 返回列表 | 待执行 |
| TC-007 | 部分失败 | 部分成功 | partial 状态 | 待执行 |
| TC-008 | 全部失败 | 全失败 | failed 状态 | 待执行 |
| TC-009 | 重试中重复 | 同时多次 | 拒绝 | 待执行 |
| TC-010 | 日志脱敏 | 手机号 | 中间四位 | 待执行 |
| TC-011 | 统计查询 | 7 天 | 统计结果 | 待执行 |
| TC-012 | 关键字搜索 | 错误内容 | 命中 | 待执行 |
| TC-013 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |
| TC-014 | 同步明细 | 100 条 | 全部展示 | 待执行 |
| TC-015 | 外部数据翻页 | 多页 | 正确 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 日志保留 | SYNC_LOG_RETENTION_DAYS | 90 | |
| 外部查询超时 | SYNC_EXTERNAL_TIMEOUT | 10s | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 同步失败率 | > 10% | 钉钉 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.12 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
