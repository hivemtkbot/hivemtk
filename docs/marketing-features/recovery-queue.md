# 流失挽回队列 (Recovery Queue)

> **所属模块**: marketing-automation
> **功能 slug**: `recoveryQueue`
> **文档定位**: 流失预警客户进入挽回队列，自动分配 AI 智能体执行挽回 SOP。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 流失挽回队列 |
| 功能名称(英文) | Recovery Queue |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 流失预警客户自动入队
- [x] 挽回 SOP 策略配置（召回话术/优惠券/转人工）
- [x] 自动分配 AI 智能体执行
- [x] `setupRecoveryQueueRoutes` 路由注册
- [x] `internal/controller/recovery_queue.go` 后端控制器
- [x] 挽回效果统计（挽回成功率/响应率）
- [x] 队列状态管理（pending/recovering/recovered/failed）

### 1.2 待完成内容
- [ ] 挽回策略智能推荐

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
私域客户在一段时间无互动后会逐渐流失。流失预测模型识别出高流失风险客户后，需要及时介入挽回。系统提供流失挽回队列，自动分配 AI 智能体按 SOP 执行挽回动作（召回话术、优惠券、转人工），提升挽回效率。

### 2.2 解决思路
流失预测模型输出高流失风险客户列表，自动进入挽回队列；按客户特征匹配挽回策略（话术/优惠券/转人工），分配给 AI 智能体执行；跟踪挽回结果（响应/复购），统计挽回成功率。

### 2.3 关键算法或模型
- 流失风险评分：churn_score（来自流失预测模型）
- 策略匹配：按客户分群 + 历史偏好选择挽回 SOP
- 智能体分配：按负载均衡 + 客户绑定关系分配

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | churn_score_min | float | 否 | 流失分数下限筛选 |
| 输入 | status | string | 否 | 队列状态筛选 |
| 输出 | queue_id | int64 | 是 | 队列项 ID |
| 输出 | customer_id | int64 | 是 | 客户 ID |
| 输出 | churn_score | float | 是 | 流失风险评分 |
| 输出 | recovery_strategy | string | 是 | 挽回策略 |
| 输出 | agent_id | int64 | 是 | 分配的智能体 ID |
| 输出 | status | string | 是 | 状态 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 队列入队延迟 < 5s（从预测到入队）
- 智能体分配 < 1s
- 挽回成功率 ≥ 15%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/recovery-queue/list | 队列列表 | JWT |
| GET | /api/recovery-queue/:id | 队列项详情 | JWT |
| POST | /api/recovery-queue/assign | 手动分配智能体 | JWT |
| PUT | /api/recovery-queue/:id/strategy | 更新挽回策略 | JWT |
| POST | /api/recovery-queue/:id/start | 启动挽回 | JWT |
| GET | /api/recovery-queue/stats | 挽回效果统计 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| recovery_queue | 挽回队列主表 |
| recovery_strategies | 挽回策略配置表 |
| recovery_execution_logs | 挽回执行日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| queue_id | bigint | 队列项 ID |
| customer_id | bigint | 客户 ID |
| churn_score | decimal(4,3) | 流失风险评分 |
| recovery_strategy | varchar(64) | 挽回策略（script/coupon/transfer_human） |
| agent_id | bigint | 分配的智能体 ID |
| status | varchar(16) | 状态（pending/recovering/recovered/failed） |

---

## 六、业务流程
### 6.1 主流程
1. 流失预测模型定期输出高流失风险客户
2. 客户自动进入挽回队列，状态 pending
3. 按客户特征匹配挽回策略，分配智能体
4. 智能体按 SOP 执行挽回（发送召回话术/优惠券）
5. 客户响应则标记 recovered，无响应则升级转人工
6. 统计挽回效果

### 6.2 异常处理
- 智能体不可用：重新分配其他智能体
- 客户已流失：标记 failed，停止挽回
- 挽回 SOP 执行失败：记录日志，重试 1 次

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 流失挽回队列 | /recovery-queue | recoveryQueue/List.vue |

### 7.2 关键交互
- 队列列表表格（客户、流失评分、策略、智能体、状态）
- 流失评分分布图
- 挽回策略配置弹窗
- 手动分配智能体操作
- 挽回效果统计卡片（成功率/响应率/平均耗时）
- 状态筛选 Tab（待处理/挽回中/已挽回/失败）

---

## 八、测试策略
### 8.1 单元测试
- 队列入队逻辑单测
- 策略匹配单测
- 智能体分配（负载均衡）单测

### 8.2 集成测试
- 端到端挽回流程测试（入队→分配→执行→统计）
- 挽回 SOP 执行测试
- 挽回效果统计准确性测试

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
