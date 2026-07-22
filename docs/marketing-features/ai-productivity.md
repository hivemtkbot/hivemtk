# 智能体产能 (AI Productivity)

> **所属模块**: analytics
> **功能 slug**: `aiProductivity`
> **文档定位**: 智能体产能报表，AI vs 人工接待量/响应时长/成交转化对比。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 智能体产能 |
| 功能名称(英文) | AI Productivity |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | analytics |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 智能体接待量统计（按日/周/月）
- [x] 平均响应时长统计
- [x] 成交转化率统计
- [x] AI vs 人工产能提升对比（productivity_lift）
- [x] `setupAnalyticsRoutes` 路由注册
- [x] 前端 `user-web/src/views/aiProductivity/List.vue` 产能报表
- [x] 智能体维度排名

### 1.2 待完成内容
- [ ] 产能预测（基于历史趋势）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
AI 销售智能体的核心价值在于提升人均产能、降低响应时长、提高成交转化。运营管理层需要量化对比 AI 与人工的产能差异，评估智能体 ROI，并识别高效智能体配置以推广复用。

### 2.2 解决思路
按周期（日/周/月）聚合每个智能体的接待会话数、平均响应时长、成交转化数，与同期人工坐席数据对比计算产能提升比例（productivity_lift），提供智能体排名与趋势分析。

### 2.3 关键算法或模型
- 产能提升计算：`productivity_lift = (ai_metric - human_metric) / human_metric`
- 响应时长：会话首条消息到智能体回复的时间差均值
- 成交转化：`conversion_count / sessions`

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | period | string | 是 | 周期（daily/weekly/monthly） |
| 输入 | start_date | string | 是 | 开始日期 |
| 输入 | end_date | string | 是 | 结束日期 |
| 输入 | agent_id | int64 | 否 | 智能体筛选 |
| 输出 | agent_id | int64 | 是 | 智能体 ID |
| 输出 | period | string | 是 | 周期 |
| 输出 | sessions | int64 | 是 | 接待会话数 |
| 输出 | avg_response_time | int | 是 | 平均响应时长（毫秒） |
| 输出 | conversion_count | int64 | 是 | 成交数 |
| 输出 | productivity_lift | decimal | 是 | 产能提升比例 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 报表查询 < 1s
- AI vs 人工对比查询 < 2s
- 数据导出 < 5s

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/ai-productivity/overview | 产能概览 | JWT |
| GET | /api/ai-productivity/agent-list | 智能体产能列表 | JWT |
| GET | /api/ai-productivity/compare | AI vs 人工对比 | JWT |
| GET | /api/ai-productivity/ranking | 智能体排名 | JWT |
| GET | /api/ai-productivity/export | 导出报表 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| agent_productivity_daily | 智能体产能日表 |
| human_productivity_daily | 人工坐席产能日表 |
| agent_productivity_summary | 产能汇总表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| agent_id | bigint | 智能体 ID |
| period | varchar(16) | 周期 |
| sessions | bigint | 接待会话数 |
| avg_response_time | int | 平均响应时长（毫秒） |
| conversion_count | bigint | 成交数 |
| productivity_lift | decimal(5,4) | 产能提升比例 |

---

## 六、业务流程
### 6.1 主流程
1. 会话与成交事件实时写入明细表
2. 调度任务每日凌晨聚合前一天产能数据到日表
3. 报表查询从日表/汇总表读取
4. AI vs 人工对比按相同周期 Join 计算 productivity_lift
5. 智能体排名按产能提升排序

### 6.2 异常处理
- 人工数据缺失：标注「无人工基线」，不计算 lift
- 聚合任务失败：自动重试，告警
- 数据修正：支持指定日期重算

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 智能体产能报表 | /ai-productivity | aiProductivity/List.vue |

### 7.2 关键交互
- 顶部周期选择（日/周/月）与日期范围
- 产能概览卡片（总会话数、平均响应、总成交、平均 lift）
- AI vs 人工对比折线图
- 智能体产能排名表格
- 导出按钮

---

## 八、测试策略
### 8.1 单元测试
- 产能提升计算单测
- 周期聚合逻辑单测
- 排名排序单测

### 8.2 集成测试
- 报表查询端到端测试
- AI vs 人工对比数据准确性测试
- 聚合任务幂等性测试

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
