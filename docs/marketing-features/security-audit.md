# 安全审计 (Security Audit)

> **所属模块**: system-management
> **功能 slug**: `securityAudit`
> **文档定位**: 安全审计面板，异常登录检测 + 权限变更追溯 + 敏感操作记录。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 安全审计 |
| 功能名称(英文) | Security Audit |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system-management |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] 异常登录检测（异常 IP/时段/频率）
- [x] 权限变更追溯
- [x] 敏感操作记录
- [x] `setupQualityRoutes` 路由注册
- [x] `internal/controller/security_audit_controller.go` 后端控制器
- [x] 告警分级（high/medium/low）
- [x] 前端 `user-web/src/views/securityAudit/List.vue` 审计面板
- [x] 告警通知（站内 + 钉钉）

### 1.2 待完成内容
- [ ] 基于机器学习的异常行为检测

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
系统承载客户敏感数据与营销资金，安全风险包括账号被盗、异常登录、越权操作、敏感数据泄露等。需要实时检测异常行为、追溯权限变更、记录敏感操作，及时告警处置。

### 2.3 关键算法或模型
- 异常 IP 检测：异地登录 + 新 IP + 黑名单 IP 匹配
- 异常时段检测：非工作时间（22:00-06:00）登录
- 异常频率检测：1 分钟内登录失败 ≥ 5 次
- 告警分级：按风险类型与影响范围分级

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | type | string | 否 | 告警类型筛选 |
| 输入 | severity | string | 否 | 严重程度筛选 |
| 输入 | user_id | int64 | 否 | 用户筛选 |
| 输入 | start_time | timestamp | 否 | 开始时间 |
| 输出 | alert_id | int64 | 是 | 告警 ID |
| 输出 | type | string | 是 | 告警类型 |
| 输出 | user_id | int64 | 是 | 用户 ID |
| 输出 | detail | object | 是 | 告警详情 |
| 输出 | severity | string | 是 | 严重程度 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 告警检测延迟 < 2s
- 审计查询 < 500ms
- 告警通知送达 < 5s

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/security-audit/alerts | 告警列表 | JWT |
| GET | /api/security-audit/alerts/:id | 告警详情 | JWT |
| POST | /api/security-audit/alerts/:id/handle | 处置告警 | JWT |
| GET | /api/security-audit/login-anomalies | 异常登录列表 | JWT |
| GET | /api/security-audit/permission-changes | 权限变更列表 | JWT |
| GET | /api/security-audit/sensitive-ops | 敏感操作列表 | JWT |
| GET | /api/security-audit/export | 导出审计报告 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| security_alerts | 安全告警主表 |
| login_anomaly_logs | 异常登录日志表 |
| permission_change_logs | 权限变更日志表 |
| sensitive_op_logs | 敏感操作日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| alert_id | bigint | 告警 ID |
| type | varchar(32) | 类型（login_anomaly/permission_change/sensitive_op） |
| user_id | bigint | 用户 ID |
| detail | jsonb | 告警详情 |
| severity | varchar(16) | 严重程度（high/medium/low） |
| status | varchar(16) | 处置状态（pending/handled/ignored） |

---

## 六、业务流程
### 6.1 主流程
1. 登录/权限变更/敏感操作事件实时写入对应日志表
2. 规则引擎分析事件，命中规则生成告警
3. 告警按严重程度分级，写入 security_alerts
4. 高危告警实时推送站内消息 + 钉钉通知
5. 安全负责人在审计面板查看、处置告警

### 6.2 异常处理
- 规则引擎故障：事件先落库，规则恢复后回扫
- 通知失败：重试 3 次，降级仅站内消息
- 误报：支持标记为误报并优化规则

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 安全审计面板 | /security-audit | securityAudit/List.vue |

### 7.2 关键交互
- 顶部告警概览卡片（high/medium/low 计数）
- Tab 切换（异常登录/权限变更/敏感操作）
- 告警列表表格（类型、用户、详情、严重程度、状态）
- 告警详情弹窗与处置操作（标记已处理/误报）
- 时间范围与多维度筛选器
- 导出审计报告按钮

---

## 八、测试策略
### 8.1 单元测试
- 异常 IP/时段/频率检测规则单测
- 告警分级逻辑单测
- 处置状态流转单测

### 8.2 集成测试
- 端到端告警生成与通知测试
- 审计查询多维度筛选测试
- 高并发事件下规则引擎稳定性测试
