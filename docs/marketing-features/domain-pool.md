# 域名池管理 (Domain Pool)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `domain-pool`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 域名池管理 |
| 功能名称（英文） | Domain Pool |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | domain-pool |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（domain_pools / domain_health_checks）
- [x] 后端 Service 与 Controller
- [x] 域名 CRUD
- [x] 健康检测（可用性/被墙/被封）
- [x] 批量检测
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

短链服务依赖多个域名。一个域名被封/被墙/到期会导致短链失效。域名池管理多个域名 + 自动健康检测 + 故障转移。

### 2.3 关键算法或模型

- 健康检测：HTTP HEAD 200 OK + DNS 解析
- 故障判定：连续 3 次失败标记异常

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | domain | string | 是 | 域名 |
| 输入 | ssl_enabled | bool | 默认 true | SSL 启用 |
| 输入 | remark | text | 否 | 备注 |
| 输出 | domain_id | int64 | 是 | 域名ID |
| 输出 | status | string | 是 | healthy/warning/blocked/expired |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/domainpool | 域名列表 |
| POST | /api/domainpool | 添加域名 |
| PUT | /api/domainpool/:id | 更新 |
| DELETE | /api/domainpool/:id | 删除 |
| POST | /api/domainpool/:id/check | 单域名检测 |
| POST | /api/domainpool/batch-check | 批量检测 |
| GET | /api/domainpool/:id/health-history | 健康历史 |

### 3.3 安全与合规

- 域名所有权校验（DNS TXT）
- HTTPS 证书检测
- 异常告警

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 单域名检测 | < 5s | ~2s |
| 批量检测 | 50 域名/批 | 50 域名/批 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/domainpool.go` | 接口 |
| Service | `internal/service/domainpool_service.go` | 业务 |
| Repository | `internal/repository/domainpool_repo.go` | 数据 |
| Model | `internal/model/domain_pool.go` | 模型 |
| Infra | `internal/cron/` + HTTP Client | 定时检测 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| short-link | 短链使用 |
| livecode | 活码使用 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 监控告警 | 异常推送 |

### 4.4 数据流向

```text
[商户] → 添加域名
   → [domainpool_service.Create]
   → DNS TXT 校验
   → 写 domain_pools
   → 触发首次检测
   → 定时任务每日批量检测
   → 写 domain_health_checks
   → 异常 → 告警 + 故障转移
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加域名（需配置 DNS TXT 验证）
2. 查看域名状态
3. 触发健康检测
4. 查看健康历史
5. 处理异常域名

### 5.2 系统处理流程

1. 鉴权
2. DNS TXT 校验
3. 写库
4. 触发检测
5. 定时任务每日批量检测
6. 异常告警

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| DNS 校验失败 | 400101 | 拒绝添加 |
| 域名被墙 | - | 标记 blocked |
| SSL 过期 | - | 告警 |

---

## 六、数据库设计

### 6.1 核心表 domain_pools

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| domain | varchar(128) | UNIQUE | 域名 |
| ssl_enabled | tinyint | 默认 1 | SSL |
| ssl_expire_at | timestamp | | SSL 过期 |
| status | varchar(16) | 非空 | healthy/warning/blocked/expired |
| remark | text | | 备注 |
| last_check_at | timestamp | | 最后检测时间 |

### 6.2 核心表 domain_health_checks

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| domain_id | bigint | FK | 域名 |
| check_time | timestamp | 非空 | 检测时间 |
| http_status | int | | HTTP 状态 |
| dns_status | varchar(16) | | DNS 状态 |
| ssl_status | varchar(16) | | SSL 状态 |
| block_status | varchar(16) | | 被墙状态 |
| latency_ms | int | | 延迟 |
| error_msg | text | | 错误信息 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 添加域名 | 合法域名 | domain_id | ✅ |
| TC-002 | DNS 校验失败 | 错误 TXT | 400101 | ✅ |
| TC-003 | 健康检测 | 域名 | 状态结果 | ✅ |
| TC-004 | 批量检测 | 50 域名 | 50 条记录 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| CHECK_INTERVAL_HOURS | CHECK_INTERVAL_HOURS | 24 |
| CHECK_TIMEOUT_SEC | CHECK_TIMEOUT_SEC | 5 |
| BATCH_CHECK_SIZE | BATCH_CHECK_SIZE | 50 |

---

## 九、参考资料

- [shortlink-management.md](shortlink-management.md)
- [livecode-management.md](livecode-management.md)
