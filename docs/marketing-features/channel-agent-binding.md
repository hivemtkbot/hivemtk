# 渠道账号绑定智能体 (Channel Agent Binding)

> **所属模块**: multi-ai-agent
> **功能 slug**: `channelAgentBinding`
> **文档定位**: 将智能体绑定到具体渠道账号（企微/抖音/短信等），实现渠道消息路由到指定智能体。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 渠道账号绑定智能体 |
| 功能名称(英文) | Channel Agent Binding |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | multi-ai-agent |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 渠道账号与智能体绑定关系 CRUD
- [x] 支持多渠道类型（wecom/douyin/sms/whatsapp/telegram 等）
- [x] 优先级配置（同一账号多智能体路由）
- [x] 绑定状态启停
- [x] `ChannelAgentBindingController` + `service.NewChannelAgentBindingService`
- [x] 消息路由层根据绑定关系分发
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 绑定关系可视化拓扑图
- [ ] 按时段自动切换智能体绑定

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
商户在多渠道（企微/抖音/短信/WhatsApp/Telegram 等）部署账号后，需要将渠道消息路由到指定智能体处理。不同渠道、不同账号可绑定不同智能体，实现渠道级智能体编排。

### 2.2 解决思路
- 建立渠道账号与智能体的绑定关系表
- 通过 `priority` 支持同账号多智能体路由（高优先级先匹配）
- 消息接入时按 channel_type + channel_account_id 查询绑定关系
- 绑定状态可启停，无需删除即可临时切换

### 2.3 关键算法或模型
- 渠道账号 → 智能体路由表查找（channel_type + account_id + priority）
- 优先级排序与回退策略

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | agent_id | int64 | 是 | 智能体 ID |
| 输入 | channel_type | string | 是 | 渠道类型 |
| 输入 | channel_account_id | int64 | 是 | 渠道账号 ID |
| 输入 | priority | int | 否 | 优先级（默认 0） |
| 输入 | status | string | 是 | 状态 |
| 输出 | id | int64 | 是 | 绑定关系 ID |
| 输出 | routed_agent_id | int64 | 否 | 路由命中的智能体 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 路由查找 < 10ms（带缓存）
- 单账号绑定关系上限 10
- 缓存命中率 ≥ 95%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/channel-agent-bindings | 绑定关系列表 | JWT |
| POST | /api/channel-agent-bindings | 创建绑定 | JWT |
| PUT | /api/channel-agent-bindings/:id | 更新绑定 | JWT |
| DELETE | /api/channel-agent-bindings/:id | 删除绑定 | JWT |
| POST | /api/channel-agent-bindings/:id/toggle | 启停绑定 | JWT |
| GET | /api/channel-agent-bindings/resolve | 路由解析（内部） | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| channel_agent_bindings | 渠道账号与智能体绑定关系 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| agent_id | bigint | 智能体 ID |
| channel_type | varchar(32) | 渠道类型 |
| channel_account_id | bigint | 渠道账号 ID |
| priority | int | 优先级 |
| status | varchar(16) | 状态 |

---

## 六、业务流程
### 6.1 主流程
1. 商户在渠道账号管理页选择账号
2. 点击「绑定智能体」，选择智能体并设置优先级
3. 系统建立绑定关系并刷新路由缓存
4. 渠道消息接入时按 channel_type + account_id 查找绑定
5. 命中智能体后转发消息进入智能体处理
6. 支持临时启停绑定（不删除关系）

### 6.2 异常处理
- 智能体已删除：返回 404，提示重新选择
- 账号已被同优先级智能体绑定：返回 409，提示调整优先级
- 路由缓存未命中：回源数据库并回填缓存

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 绑定关系列表 | /channelAgentBinding/list | channelAgentBinding/List.vue |
| 绑定管理（渠道账号详情内嵌） | /channelAccount/:id/bindings | channelAccount/Bindings.vue |

### 7.2 关键交互
- 列表按渠道类型分组展示
- 绑定操作支持拖拽排序调整优先级
- 启停切换实时生效（带确认弹窗）
- 删除绑定前提示影响范围

---

## 八、测试策略
### 8.1 单元测试
- 绑定关系 CRUD service 单测
- 优先级路由解析单测
- 缓存命中/未命中场景单测

### 8.2 集成测试
- 创建绑定→消息路由→智能体调用全链路
- 启停绑定后路由实时切换验证
- 跨渠道账号绑定隔离验证

---

## 九、版本历史
| 版本 | 日期 | 变更说明 |
|---|---|---|
| v1.0 | 2026-07-15 | 初始实现 |
| v1.1 | 2026-07-22 | 补充功能文档 |

---

## 十、相关文档
- [../INDEX.md](../INDEX.md)
- [../architecture/ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- [../CROSS_COMPARISON_REPORT.md](../CROSS_COMPARISON_REPORT.md)
