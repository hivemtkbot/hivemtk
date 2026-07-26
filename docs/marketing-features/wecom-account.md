# 企微账号管理 (WeCom Account)

> **所属模块**: community-management
> **功能 slug**: `wecomAccount`
> **文档定位**: 管理多个企业微信账号（corpid/secret/agent_id），监控账号健康度（Token 有效性/API 限流/联系人同步状态）。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 企微账号管理 |
| 功能名称(英文) | WeCom Account |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | community-management |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 多企微账号管理（corpid/secret/agent_id）
- [x] secret 加密存储
- [x] 账号健康度监控（Token 有效性/API 限流/联系人同步状态）
- [x] `setupWeComRoutes` + `setupWeComHealthRoutes`
- [x] 前端账号列表与健康看板
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 账号健康度自动告警（钉钉/飞书）
- [ ] 多账号负载均衡

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
商户拥有多个企业微信账号（不同主体/不同业务线），需要集中管理 corpid/secret/agent_id，并监控每个账号的健康度（Token 是否有效、API 是否限流、联系人是否同步成功）。

### 2.3 关键算法或模型
- Token 有效性检测（定时调用企微 API）
- API 限流检测（响应头 + 错误码）
- health_score 加权计算

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | corp_id | string | 是 | 企业 ID |
| 输入 | agent_id | int64 | 是 | 应用 ID |
| 输入 | secret | string | 是 | 应用 secret（加密存储） |
| 输出 | account_id | int64 | 是 | 账号 ID |
| 输出 | status | string | 是 | 状态 |
| 输出 | health_score | int | 否 | 健康评分 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 健康检查间隔 5 分钟
- Token 刷新提前量 10 分钟
- health_score 计算耗时 < 500ms

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/wecom/accounts | 账号列表 | JWT |
| POST | /api/wecom/accounts | 创建账号 | JWT |
| GET | /api/wecom/accounts/:id | 账号详情 | JWT |
| PUT | /api/wecom/accounts/:id | 更新账号 | JWT |
| DELETE | /api/wecom/accounts/:id | 删除账号 | JWT |
| GET | /api/wecom/accounts/:id/health | 健康详情 | JWT |
| POST | /api/wecom/accounts/:id/sync-contacts | 触发联系人同步 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| wecom_accounts | 企微账号 |
| wecom_health_logs | 健康检查日志 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| account_id | bigint | 账号 ID |
| corp_id | varchar(64) | 企业 ID |
| agent_id | int64 | 应用 ID |
| secret | varchar(256) | 应用 secret（加密） |
| status | varchar(16) | 状态 |
| health_score | int | 健康评分 |

---

## 六、业务流程
### 6.1 主流程
1. 商户添加企微账号（corp_id + agent_id + secret）
2. 系统加密存储 secret
3. 定时健康检查：Token 有效性 / API 限流 / 联系人同步
4. 计算 health_score 并更新账号状态
5. 异常状态触发告警
6. 支持手动触发联系人同步

### 6.2 异常处理
- Token 失效：自动刷新，刷新失败标记异常
- API 限流：降级请求频率，记录限流日志
- 联系人同步失败：记录失败原因，支持重试
- secret 解密失败：标记账号不可用，告警

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 账号列表 | /wecomAccount/list | wecomAccount/List.vue |
| 健康看板 | /wecomAccount/health | wecomAccount/Health.vue |

### 7.2 关键交互
- 列表展示账号状态与 health_score
- 添加账号时支持 secret 掩码显示
- 健康看板展示 Token/限流/同步三维度详情
- 手动触发同步按钮带进度展示

---

## 八、测试策略
### 8.1 单元测试
- 账号 CRUD service 单测
- secret 加解密单测
- health_score 计算单测

### 8.2 集成测试
- 添加账号→健康检查→状态更新全链路
- Token 失效自动刷新验证
- 联系人同步触发与重试验证
