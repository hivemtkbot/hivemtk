# 触达 Pipeline 框架 (Reach Pipeline)

> **所属模块**: reach-center
> **功能 slug**: `reachPipeline`
> **文档定位**: 统一触达执行框架，9 步流水线（Trigger → Audience → Channel → Compliance → Content → Render → Send → Audit → Track），每步可降级/重试/兜底。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 触达 Pipeline 框架 |
| 功能名称(英文) | Reach Pipeline |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | reach-center |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 9 步流水线编排（Trigger → Audience → Channel → Compliance → Content → Render → Send → Audit → Track）
- [x] 每步支持降级 / 重试 / 兜底策略
- [x] DAG 流水线 + 装饰器链（限流 / 重试 / 审计 / 计费）
- [x] 触发类型与受众筛选配置
- [x] `internal/controller/reach_pipeline_controller.go` + `setupReachPipelineRoutes`
- [x] 前端流水线列表与编排页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 流水线可视化 DAG 拖拽编排
- [ ] 跨流水线 A/B 对比报表

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
营销触达场景多样（短信/邮件/企微/IM），每类触达都涉及触发、受众、渠道、合规、内容、渲染、发送、审计、追踪等环节。统一 Pipeline 框架将触达流程标准化，支持每步可降级、重试、兜底。

### 2.2 解决思路
- 9 步流水线标准化：Trigger → Audience → Channel → Compliance → Content → Render → Send → Audit → Track
- DAG 流水线调度，步骤间可并行/串行
- 装饰器链：限流、重试、审计、计费横切关注点
- 每步可配置降级策略与兜底动作

### 2.3 关键算法或模型
- DAG 流水线调度算法
- 装饰器链模式（限流 / 重试 / 审计 / 计费）
- 降级与兜底策略（fallback chain）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 流水线名称 |
| 输入 | steps_config | array | 是 | 9 步配置 |
| 输入 | trigger_type | string | 是 | 触发类型 |
| 输入 | audience_filter | object | 是 | 受众筛选 |
| 输入 | status | string | 是 | 状态 |
| 输出 | id | int64 | 是 | 流水线 ID |
| 输出 | run_id | int64 | 否 | 执行实例 ID |
| 输出 | step_results | array | 否 | 各步执行结果 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 单流水线编排加载 < 200ms
- 单步执行超时默认 30s（可配置）
- 并发流水线执行上限 50

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/reach-pipelines | 流水线列表 | JWT |
| POST | /api/reach-pipelines | 创建流水线 | JWT |
| GET | /api/reach-pipelines/:id | 流水线详情 | JWT |
| PUT | /api/reach-pipelines/:id | 更新流水线 | JWT |
| DELETE | /api/reach-pipelines/:id | 删除流水线 | JWT |
| POST | /api/reach-pipelines/:id/run | 触发执行 | JWT |
| GET | /api/reach-pipelines/:id/runs/:run_id | 执行实例详情 | JWT |
| GET | /api/reach-pipelines/:id/runs/:run_id/steps | 各步执行结果 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| reach_pipelines | 流水线定义 |
| reach_pipeline_runs | 流水线执行实例 |
| reach_pipeline_step_results | 各步执行结果 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| name | varchar(128) | 流水线名称 |
| steps_config | jsonb | 9 步配置 |
| status | varchar(16) | 状态 |
| trigger_type | varchar(32) | 触发类型 |
| audience_filter | jsonb | 受众筛选 |

---

## 六、业务流程
### 6.1 主流程
1. 商户创建触达流水线
2. 配置 9 步：触发方式、受众筛选、渠道选择、合规校验、内容模板、渲染、发送、审计、追踪
3. 每步配置降级/重试/兜底策略
4. 触发执行（手动/定时/事件）
5. Pipeline 调度器按 DAG 执行各步
6. 装饰器链记录限流/重试/审计/计费
7. 失败步骤按策略降级或兜底
8. 全程输出各步执行结果

### 6.2 异常处理
- 受众筛选为空：终止流水线，记录告警
- 合规校验失败：跳过发送，记录违规
- 渠道限流：触发重试或降级到备用渠道
- 发送失败：按重试策略重试，超限后兜底

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 流水线列表 | /reachPipeline/list | reachPipeline/List.vue |
| 流水线编排 | /reachPipeline/edit/:id | reachPipeline/Edit.vue |
| 执行实例详情 | /reachPipeline/run/:run_id | reachPipeline/Run.vue |

### 7.2 关键交互
- 列表按触发类型、状态筛选
- 编排页支持步骤启用/禁用
- 执行实例展示各步状态、耗时、日志
- 失败步骤支持手动重试

---

## 八、测试策略
### 8.1 单元测试
- DAG 调度器单测
- 装饰器链单测（限流/重试/审计/计费）
- 降级与兜底策略单测

### 8.2 集成测试
- 创建→触发→执行→追踪全链路
- 渠道限流降级到备用渠道验证
- 合规校验失败终止发送验证

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
