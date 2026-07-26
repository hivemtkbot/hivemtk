# SOP 智能体 (SOP Agent)

> **所属模块**: ai-agent-core
> **功能 slug**: `sopAgent`
> **文档定位**: 销冠 SOP 可视化编排（Greet → Probe → Value → Objection → Closing），AI 根据客户反馈自动流转节点，检测循环超 3 次自动转人工。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | SOP 智能体 |
| 功能名称(英文) | SOP Agent |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 销冠 SOP 可视化编排（Greet → Probe → Value → Objection → Closing）
- [x] 状态机 DAG 流转
- [x] 每节点绑定 SOP 节点配置 + 触发条件 + 升级规则
- [x] 客户反馈驱动自动流转节点
- [x] 循环检测超 3 次自动转人工
- [x] `internal/controller/sop_controller.go` + `setupSOPRoutes`
- [x] 前端 SOP 列表与编排页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] SOP 模板市场
- [ ] SOP 效果转化漏斗分析

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
销冠流程标准化为 SOP（Greet → Probe → Value → Objection → Closing），AI 根据客户反馈自动流转节点，实现可复制的销冠话术。当客户在同一节点循环超过 3 次时，自动转人工避免僵局。

### 2.3 关键算法或模型
- 状态机 DAG 流转算法
- 循环检测（handoff_count ≥ 3 触发转人工）
- 节点触发条件匹配（意图 + 关键词 + 置信度）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | SOP 名称 |
| 输入 | nodes | array | 是 | 节点列表 |
| 输入 | edges | array | 是 | 边列表 |
| 输入 | status | string | 是 | 状态 |
| 输出 | id | int64 | 是 | SOP ID |
| 输出 | current_node | string | 否 | 当前节点 |
| 输出 | handoff_count | int | 否 | 循环计数 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 节点流转判定 < 100ms
- 单 SOP 节点上限 50
- 循环检测阈值默认 3（可配置）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/sops | SOP 列表 | JWT |
| POST | /api/sops | 创建 SOP | JWT |
| GET | /api/sops/:id | SOP 详情 | JWT |
| PUT | /api/sops/:id | 更新 SOP | JWT |
| DELETE | /api/sops/:id | 删除 SOP | JWT |
| POST | /api/sops/:id/advance | 流转节点 | JWT |
| GET | /api/sops/:id/sessions/:sid | 会话状态 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| sops | SOP 定义 |
| sop_nodes | SOP 节点 |
| sop_edges | SOP 边 |
| sop_sessions | SOP 会话状态 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| name | varchar(128) | SOP 名称 |
| nodes | jsonb | 节点列表 |
| edges | jsonb | 边列表 |
| current_node | varchar(64) | 当前节点 |
| status | varchar(16) | 状态 |
| handoff_count | int | 循环计数 |

---

## 六、业务流程
### 6.1 主流程
1. 商户创建 SOP，编排节点（Greet → Probe → Value → Objection → Closing）
2. 每节点配置话术、触发条件、升级规则
3. 客户消息进入，意图识别 + 当前节点配置判定流转
4. 命中触发条件 → 流转到下一节点
5. 未命中 → 停留当前节点，handoff_count +1
6. handoff_count ≥ 3 → 自动转人工
7. 流转至 Closing 节点 → 标记成交

### 6.2 异常处理
- 节点配置缺失：返回 400，提示补全配置
- 流转死锁：超时自动转人工
- 客户主动结束：会话状态置为 ended

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| SOP 列表 | /sopAgent/list | sopAgent/List.vue |
| SOP 编排 | /sopAgent/edit/:id | sopAgent/Edit.vue |
| 会话状态查看 | /sopAgent/session/:sid | sopAgent/Session.vue |

### 7.2 关键交互
- 编排页支持节点拖拽与连线
- 节点配置面板支持话术预览
- 会话状态展示当前节点与流转历史
- 循环计数实时显示，接近阈值高亮告警

---

## 八、测试策略
### 8.1 单元测试
- 状态机 DAG 流转单测
- 循环检测与转人工单测
- 节点触发条件匹配单测

### 8.2 集成测试
- 创建 SOP → 模拟客户消息 → 流转全链路
- 循环 3 次自动转人工验证
- 流转至 Closing 标记成交验证
