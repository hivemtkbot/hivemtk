# 转化漏斗 (Conversion Funnel)

> **所属模块**: analytics
> **功能 slug**: `conversionFunnel`
> **文档定位**: 多渠道转化漏斗分析，按渠道/时间/智能体维度对比转化效率。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 转化漏斗 |
| 功能名称(英文) | Conversion Funnel |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | analytics |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 5 阶段转化漏斗（曝光→点击→咨询→线索→成交）
- [x] `setupAnalyticsRoutes` 路由注册
- [x] 多维度对比（渠道/时间/智能体）
- [x] 阶段转化率与平均耗时统计
- [x] 前端 `user-web/src/views/conversionFunnel/List.vue` 漏斗分析页
- [x] 漏斗数据导出（CSV/Excel）

### 1.2 待完成内容
- [ ] 自定义漏斗阶段配置

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
私域销售从曝光到成交经历多个转化环节，不同渠道、不同智能体的转化效率差异显著。运营人员需要量化每个环节的转化率与流失率，定位转化瓶颈，优化投放与承接策略。

### 2.2 解决思路
基于客户行为事件流构建标准化 5 阶段漏斗（曝光→点击→咨询→线索→成交），按渠道/时间/智能体维度聚合统计各阶段客户数、阶段间转化率与平均耗时，通过对比分析定位低效环节。

### 2.3 关键算法或模型
- 阶段归属判定：按客户首次进入该阶段的时间戳
- 转化率：`conversion_rate = next_stage_count / current_stage_count`
- 平均耗时：`avg_time = avg(next_stage_enter - current_stage_enter)`
- 多维度聚合：按 channel / agent_id / date 分组

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | start_date | string | 是 | 开始日期 |
| 输入 | end_date | string | 是 | 结束日期 |
| 输入 | dimension | string | 否 | 对比维度（channel/agent/date） |
| 输出 | stage | string | 是 | 阶段名称 |
| 输出 | channel | string | 是 | 渠道 |
| 输出 | count | int64 | 是 | 阶段客户数 |
| 输出 | conversion_rate | float | 是 | 转化率 |
| 输出 | avg_time | int | 是 | 平均耗时（秒） |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 漏斗查询响应 < 1s（百万级事件数据）
- 多维度对比查询 < 2s
- 数据导出 < 5s（10 万行）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/conversion-funnel/overview | 漏斗概览 | JWT |
| GET | /api/conversion-funnel/compare | 多维度对比 | JWT |
| GET | /api/conversion-funnel/stage-detail | 阶段详情 | JWT |
| GET | /api/conversion-funnel/export | 导出漏斗数据 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| funnel_stage_events | 漏斗阶段事件表 |
| funnel_aggregations | 漏斗聚合结果表（预计算） |
| funnel_configs | 漏斗配置表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| stage | varchar(32) | 阶段（impression/click/consult/lead/deal） |
| channel | varchar(32) | 渠道 |
| count | bigint | 阶段客户数 |
| conversion_rate | decimal(5,4) | 转化率 |
| avg_time | int | 平均耗时（秒） |

---

## 六、业务流程
### 6.1 主流程
1. 客户行为事件实时写入 funnel_stage_events
2. 调度任务每小时聚合各维度漏斗数据到 funnel_aggregations
3. 前端发起漏斗查询，优先读取聚合表
4. 支持按渠道/时间/智能体维度对比
5. 用户可下钻查看阶段客户明细

### 6.2 异常处理
- 阶段事件缺失：客户不计入后续阶段
- 聚合任务失败：自动重试 3 次，告警
- 数据回补：支持指定日期范围重算

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 转化漏斗分析 | /conversion-funnel | conversionFunnel/List.vue |

### 7.2 关键交互
- 顶部时间范围与对比维度选择器
- 漏斗图展示各阶段客户数与转化率
- 多维度对比柱状图（渠道/智能体对比）
- 阶段点击下钻查看客户明细
- 导出按钮（CSV/Excel）

---

## 八、测试策略
### 8.1 单元测试
- 转化率与平均耗时计算单测
- 多维度聚合逻辑单测
- 阶段归属判定单测

### 8.2 集成测试
- 漏斗查询端到端测试
- 聚合任务幂等性测试
- 大数据量导出性能测试

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
