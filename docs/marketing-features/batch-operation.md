# 批量操作 (Batch Operation)

> **所属模块**: marketing-automation
> **功能 slug**: `batchOperation`
> **文档定位**: 批量客户操作（打标/发消息/分配销售/导入导出），任务队列 + 进度跟踪。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 批量操作 |
| 功能名称(英文) | Batch Operation |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 批量操作类型（打标/发消息/分配销售/导入/导出）
- [x] `setupBatchRoutes` 路由注册
- [x] 任务队列与 Worker 调度
- [x] 实时进度跟踪（processed_count / target_count）
- [x] 失败重试与错误明细
- [x] 前端 `user-web/src/views/batchOperation/List.vue` 任务管理
- [x] 操作结果导出

### 1.2 待完成内容
- [ ] 批量操作定时调度

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
私域运营常需对大量客户执行相同操作（如批量打标、群发消息、分配销售）。手动逐个操作效率低下，需要支持批量任务，并通过队列削峰、进度可视化、失败重试保障执行可靠性。

### 2.2 解决思路
用户提交批量任务（操作类型 + 目标客户集 + 操作参数），任务入队后由 Worker 并发消费执行；实时更新 processed_count 与状态，前端轮询/SSE 推送进度；失败项记录明细支持重试。

### 2.3 关键算法或模型
- 任务分片：大任务按 100 条/片拆分子任务并发执行
- 限流：按操作类型配置 QPS（如发消息 10 QPS）
- 重试策略：失败重试 3 次，指数退避

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | operation_type | string | 是 | 操作类型 |
| 输入 | target_customers | array | 是 | 目标客户 ID 列表 |
| 输入 | operation_params | object | 是 | 操作参数 |
| 输出 | batch_id | int64 | 是 | 批次 ID |
| 输出 | target_count | int64 | 是 | 目标总数 |
| 输出 | processed_count | int64 | 是 | 已处理数 |
| 输出 | status | string | 是 | 状态 |
| 输出 | result | object | 是 | 结果汇总 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 单任务提交 < 500ms
- Worker 并发度 ≥ 10
- 万级客户批量操作 < 10 分钟

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/batch-operation/list | 批次列表 | JWT |
| POST | /api/batch-operation | 提交批量任务 | JWT |
| GET | /api/batch-operation/:id | 批次详情与进度 | JWT |
| POST | /api/batch-operation/:id/retry | 重试失败项 | JWT |
| POST | /api/batch-operation/:id/cancel | 取消批次 | JWT |
| GET | /api/batch-operation/:id/result | 导出结果 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| batch_operations | 批次主表 |
| batch_operation_items | 批次明细项表 |
| batch_operation_logs | 操作日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| batch_id | bigint | 批次 ID |
| operation_type | varchar(32) | 操作类型（tag/message/assign/import/export） |
| target_count | bigint | 目标总数 |
| processed_count | bigint | 已处理数 |
| status | varchar(16) | 状态（pending/running/completed/failed/cancelled） |
| result | jsonb | 结果汇总 |

---

## 六、业务流程
### 6.1 主流程
1. 用户在批量操作页选择操作类型与目标客户集
2. 提交任务，系统生成 batch_id 入队
3. Worker 消费任务，按分片并发执行
4. 每处理一项更新 processed_count
5. 前端轮询进度，完成后展示结果汇总
6. 失败项可一键重试

### 6.2 异常处理
- 任务超时：标记 failed，支持续跑
- 单项失败：记录错误明细，继续处理其他项
- 系统重启：任务状态为 running 的标记为 interrupted，支持恢复

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 批量操作管理 | /batch-operation | batchOperation/List.vue |

### 7.2 关键交互
- 批次列表表格（类型、目标数、进度条、状态）
- 提交任务向导（选类型→选客户→配置参数→确认）
- 实时进度条与已处理/失败数展示
- 失败项明细弹窗与重试按钮
- 结果导出按钮

---

## 八、测试策略
### 8.1 单元测试
- 任务分片逻辑单测
- 限流与重试策略单测
- 状态机流转单测

### 8.2 集成测试
- 端到端批量操作测试（各操作类型）
- Worker 并发与幂等性测试
- 任务恢复（中断后续跑）测试

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
