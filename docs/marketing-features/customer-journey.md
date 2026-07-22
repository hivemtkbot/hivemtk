# 客户旅程大屏 (Customer Journey Dashboard)

> **所属模块**: analytics
> **功能 slug**: `customerJourney`
> **文档定位**: 9 阶段客户旅程实时监控与漏斗转化分析大屏。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 客户旅程大屏 |
| 功能名称(英文) | Customer Journey Dashboard |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | analytics |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 9 阶段客户旅程模型（认知→兴趣→考虑→意向→对比→决策→购买→使用→复购）
- [x] 实时阶段客户数统计与漏斗转化率计算
- [x] 阶段平均停留时长统计
- [x] 大屏可视化（漏斗图、桑基图、阶段卡片）
- [x] `setupCustomerJourneyRoutes` 路由注册
- [x] `internal/controller/customer_journey_controller.go` 后端控制器
- [x] 前端 `user-web/src/views/customerJourney/Dashboard.vue` 大屏视图
- [x] 多维度筛选（时间范围、智能体、渠道）

### 1.2 待完成内容
- [ ] 单客户旅程时间轴回放

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
私域销售场景中客户从首次接触到最终复购会经历多个心理与行为阶段。运营人员需要一张可实时观察各阶段客户存量、转化效率与停留时长的全景大屏，以快速定位转化瓶颈、优化阶段策略。

### 2.2 解决思路
基于客户行为事件与标签计算每个客户当前所处旅程阶段，按阶段聚合统计客户数、阶段间转化率与平均停留时长，通过 ECharts 漏斗图与桑基图实时可视化，支持按智能体/渠道/时间维度下钻。

### 2.3 关键算法或模型
- 阶段判定规则引擎：基于最近行为事件 + 标签权重计算当前阶段
- 漏斗转化率：`conversion_rate = next_stage_count / current_stage_count`
- 阶段停留时长：`avg_duration = avg(stage_enter_time - next_stage_enter_time)`

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | start_date | string | 是 | 统计开始日期 |
| 输入 | end_date | string | 是 | 统计结束日期 |
| 输入 | agent_id | int64 | 否 | 智能体筛选 |
| 输入 | channel | string | 否 | 渠道筛选 |
| 输出 | stage | string | 是 | 阶段名称 |
| 输出 | customer_count | int64 | 是 | 该阶段客户数 |
| 输出 | conversion_rate | float | 是 | 下一阶段转化率 |
| 输出 | avg_duration | int | 是 | 平均停留时长（秒） |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 大屏首屏加载 < 2s
- 阶段聚合查询 < 800ms（百万级客户数据）
- 缓存命中率 ≥ 90%（5 分钟滚动缓存）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/customer-journey/overview | 旅程概览（9 阶段汇总） | JWT |
| GET | /api/customer-journey/funnel | 漏斗转化数据 | JWT |
| GET | /api/customer-journey/stage-detail | 阶段详情（含停留时长分布） | JWT |
| GET | /api/customer-journey/export | 导出大屏数据 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| customer_journey_stages | 客户当前旅程阶段记录表 |
| customer_journey_events | 客户阶段流转事件表 |
| journey_stage_config | 阶段判定规则配置表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| stage | varchar(32) | 阶段标识（awareness/interest/...） |
| customer_count | bigint | 阶段客户数 |
| conversion_rate | decimal(5,4) | 转化率 |
| avg_duration | int | 平均停留时长（秒） |
| entered_at | timestamp | 进入阶段时间 |
| next_stage | varchar(32) | 下一阶段 |

---

## 六、业务流程
### 6.1 主流程
1. 调度任务每 5 分钟扫描客户行为事件，更新客户当前阶段
2. 大屏前端发起 `/overview` 请求获取 9 阶段汇总
3. 后端从缓存读取聚合数据，未命中则实时聚合计算后回填缓存
4. 前端渲染漏斗图与阶段卡片，点击阶段下钻查看明细
5. 用户可切换时间范围/智能体/渠道重新加载

### 6.2 异常处理
- 缓存击穿：单飞机制（singleflight）合并并发请求
- 阶段判定失败：客户归入「未知」阶段并告警
- 数据回补：支持指定日期范围重算阶段

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 客户旅程大屏 | /customer-journey | customerJourney/Dashboard.vue |

### 7.2 关键交互
- 顶部时间范围选择器（今日/近 7 天/近 30 天/自定义）
- 9 阶段卡片横排展示，点击卡片高亮漏斗对应阶段
- 漏斗图悬停显示转化率与流失数
- 桑基图展示阶段流转路径分布
- 右上角刷新按钮强制刷新缓存

---

## 八、测试策略
### 8.1 单元测试
- 阶段判定规则引擎单测（覆盖 9 阶段流转）
- 转化率与停留时长计算单测
- 缓存读写与失效单测

### 8.2 集成测试
- 大屏接口端到端测试（含缓存命中/未命中场景）
- 多维度筛选组合测试
- 大数据量（百万级）聚合性能测试

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
