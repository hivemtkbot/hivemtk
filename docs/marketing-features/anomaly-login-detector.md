# 异常登录预警 (Anomaly Login Detector)

> **所属模块**: auth（A 域 P1-2）
> **功能 slug**: `anomaly-login-detector`
> **文档定位**: 异常登录检测与告警处置，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **代码位置**: `user-server/internal/controller/anomaly_login_detector_controller.go` + `internal/service/anomalyLoginDetector.go`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 异常登录预警 |
| 功能名称(英文) | Anomaly Login Detector |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | A 域-认证安全 |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 登录事件审计表（`login_events`）与安全告警表（`security_alerts`）
- [x] 登录事件入库 + 风险评估（IP/UA/异常时段/异地等维度）
- [x] 告警状态机：`open → resolved / ignored`
- [x] 告警处置接口（处理 / 忽略 + 处理说明 + 审计日志）
- [x] 分页查询登录事件与告警
- [x] Controller 仅做参数解析 / 调 service / 统一响应（薄层 controller）

### 1.2 待完成内容

- [ ] 异常登录实时通知（邮件 / 钉钉 / 站内信）
- [ ] 设备指纹维度加强

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

私域部署环境下仍需防范异常登录：弱密码爆破、异地登录、深夜登录、UA 异常切换等。系统需自动评估每次登录的风险等级，并对 `high` / `critical` 级别事件生成告警供管理员处置。

### 2.3 关键算法或模型

- **风险评估维度**: IP 陌生度 / UA 指纹变化 / 异常时段 / 失败次数 / 历史登录地点比对
- **状态机**: `open`（待处理）→ `resolved`（已解决）或 `ignored`（已忽略）
- **审计**: 处置动作写入 `operation_logs`，记录处置人 / 动作 / 备注

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(ListLoginEvents) | page | int | 否 | 页码（默认 1） |
| 输入(ListLoginEvents) | page_size | int | 否 | 每页大小（默认 20，最大 100） |
| 输入(ListAlerts) | status | string | 否 | 状态过滤：open/resolved/ignored |
| 输入(ListAlerts) | page / page_size | int | 否 | 分页参数 |
| 输入(ResolveAlert/IgnoreAlert) | id | uint | 是 | 告警 ID（路径参数） |
| 输入(ResolveAlert/IgnoreAlert) | note | string | 否 | 处理说明 |
| 输出 | list | array | 是 | 登录事件或告警列表 |
| 输出 | total | int | 是 | 总条数 |

---

## 三、设计标准

### 3.1 API 契约

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/auth/anomaly/login-events | JWT | 查询当前用户登录事件 |
| GET | /api/auth/anomaly/alerts | JWT | 查询安全告警（支持 status 过滤） |
| POST | /api/auth/anomaly/alerts/:id/resolve | JWT | 处理告警（标记 resolved） |
| POST | /api/auth/anomaly/alerts/:id/ignore | JWT | 忽略告警（标记 ignored） |

### 3.2 安全与合规

- 仅本人可查询自身登录事件与告警（按 `user_id` 过滤）
- 处置 / 忽略动作落审计日志
- 告警 ID 解析失败返回 400
- 告警状态变更走 `service.AnomalyLoginDetector` 业务校验

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 登录事件列表查询 | < 100ms |
| 告警列表查询 | < 100ms |
| 告警处置 | < 200ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/anomaly_login_detector_controller.go | 薄层 controller |
| Service | internal/service/AnomalyLoginDetector | 风险评估 + 告警处置 |
| Repository | internal/repository（auth_security） | login_events / security_alerts |
| Model | internal/model（user_mfa / login_events / security_alerts） | 数据模型 |
| Migration | internal/migration/migrations/auth_security_migration.go | v2.10.0 表迁移 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 登录认证（auth-login-jwt） | 登录入口写入 login_events |
| 操作日志（operation-log） | 告警处置审计 |
| 用户管理（user-management） | user_id 关联 |

### 4.3 数据流向

```text
[用户登录] → [login_events 入库] → [风险评估] → [命中阈值?]
                                              ├─ 是 → [security_alerts 入库(open)]
                                              └─ 否 → [结束]
[管理员处置] → [状态变更 resolved/ignored] → [审计日志] → [返回成功]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 用户登录（成功或失败）
2. 系统自动评估风险，异常时生成告警
3. 管理员在「安全审计 → 异常登录告警」查看 open 状态告警
4. 调研后选择"处理"（resolved）或"忽略"（ignored），填备注
5. 系统更新状态并写审计日志

### 5.2 系统处理流程

1. Controller 接收请求，解析 user_id / id / note
2. 调用 `service.AnomalyLoginDetector.ResolveAlert` / `IgnoreAlert`
3. Service 校验告警存在性 / 当前状态
4. 更新 `security_alerts` 状态 + 写 `operation_logs`
5. 返回统一响应

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 告警 ID 无效 | 400 | "无效的告警 ID" |
| 告警不存在 / 状态非法 | 400 | service 返回业务错误 |
| DB 异常 | 500 | 原始错误 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `login_events` | 登录事件审计（含风险评估结果） |
| `security_alerts` | 安全告警（high/critical 触发） |

迁移文件：`internal/migration/migrations/auth_security_migration.go`（v2.10.0，P1-1 MFA / P1-2 异常登录 / P1-3 密码策略 / P1-4 行级权限）。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 登录事件入库 | 正常登录 | login_events 1 条 | 待执行 |
| TC-002 | 异常登录触发告警 | 异地登录 | security_alerts 1 条 open | 待执行 |
| TC-003 | 处理告警 | note="已确认本人" | 状态 resolved | 待执行 |
| TC-004 | 忽略告警 | note="误报" | 状态 ignored | 待执行 |
| TC-005 | 越权查询他人告警 | user_id 不匹配 | 返回 403 或空列表 | 待执行 |
| TC-006 | 无效 ID | id="abc" | 400 | 待执行 |
| TC-007 | 分页参数兜底 | page=0/page_size=999 | 默认 1/20 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

无独立环境变量，风险评估阈值在 `service.AnomalyLoginDetector` 内部默认值，后续可补后台配置。

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| open 告警堆积 | > 50 条 | 钉钉 / 邮件 |
| 登录失败率 | > 30% / 5 分钟 | 钉钉 |

---

## 九、参考资料

- `user-server/internal/controller/anomaly_login_detector_controller.go`
- `user-server/internal/migration/migrations/auth_security_migration.go`
- [auth-login-jwt.md](auth-login-jwt.md)
- [security-audit.md](security-audit.md)
- [operation-log.md](operation-log.md)
