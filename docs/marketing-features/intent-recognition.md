# 意图识别中心 (Intent Recognition)

> **所属模块**: ai-agent-core
> **功能 slug**: `intentRecognition`
> **文档定位**: 客户消息进入智能体前先识别意图，12 类意图 + 规则与 LLM 双路识别。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 意图识别中心 |
| 功能名称(英文) | Intent Recognition |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P0 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 12 类意图定义（咨询/比价/异议/成交/售后/投诉/闲聊/转人工/复购/转介绍/无效/紧急）
- [x] 规则匹配（关键词/正则）优先识别
- [x] LLM 二次分类（置信度 < 0.7 时触发）
- [x] 输出 intent_type + confidence + source
- [x] `internal/controller/intent_controller.go` + `setupIntentRoutes`
- [x] 前端意图识别记录列表
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 意图识别准确率离线评估报表
- [ ] 自定义意图扩展

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
客户消息进入智能体前需先识别意图，以决定后续路由（转 SOP / 转人工 / 直接回复 / 标记无效）。12 类意图覆盖销售全流程，规则与 LLM 双路识别兼顾速度与准确率。

### 2.2 解决思路
- 规则匹配优先：关键词 + 正则，命中即输出高置信度结果
- 置信度 < 0.7 时调用 LLM 二次分类
- 输出 intent_type、confidence、source（rule/llm）、metadata
- 识别结果驱动后续 SOP 节点流转或人工转接

### 2.3 关键算法或模型
- 规则引擎：关键词字典 + 正则模式匹配
- LLM 分类：prompt + few-shot examples + 结构化输出
- 置信度阈值：0.7（可配置）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | message_id | int64 | 是 | 消息 ID |
| 输入 | content | string | 是 | 消息内容 |
| 输入 | customer_id | int64 | 否 | 客户 ID |
| 输出 | id | int64 | 是 | 识别记录 ID |
| 输出 | intent_type | string | 是 | 12 类意图之一 |
| 输出 | confidence | float | 是 | 置信度 0-1 |
| 输出 | source | string | 是 | rule/llm |
| 输出 | metadata | object | 否 | 附加信息 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 规则识别 < 10ms
- LLM 识别 < 2s（含 LLM 调用）
- 规则命中率 ≥ 60%
- 整体准确率 ≥ 85%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/intent-recognitions | 识别记录列表 | JWT |
| POST | /api/intent-recognitions/recognize | 实时识别 | JWT |
| GET | /api/intent-recognitions/:id | 识别详情 | JWT |
| GET | /api/intent-recognitions/stats | 识别统计 | JWT |
| PUT | /api/intent-recognitions/config | 阈值与规则配置 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| intent_recognitions | 意图识别记录 |
| intent_rules | 意图识别规则（关键词/正则） |
| intent_config | 阈值与开关配置 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| message_id | bigint | 消息 ID |
| intent_type | varchar(32) | 12 类意图之一 |
| confidence | float | 置信度 |
| source | varchar(8) | rule/llm |
| metadata | jsonb | 附加信息 |

---

## 六、业务流程
### 6.1 主流程
1. 客户消息进入意图识别中心
2. 规则引擎优先匹配（关键词/正则）
3. 命中且置信度 ≥ 0.7 → 直接输出（source=rule）
4. 未命中或置信度 < 0.7 → 调用 LLM 二次分类（source=llm）
5. 输出 intent_type + confidence + source
6. 下游根据意图路由（SOP / 人工 / 直接回复 / 标记无效）

### 6.2 异常处理
- LLM 调用失败：回退为规则结果，confidence 标记降级
- 意图均为低置信度：默认路由到「转人工」
- 紧急意图：立即触发人工接管告警

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 意图识别记录 | /intentRecognition/list | intentRecognition/List.vue |
| 规则配置 | /intentRecognition/rules | intentRecognition/Rules.vue |

### 7.2 关键交互
- 列表按 intent_type、source 筛选
- 识别记录展示规则命中详情与 LLM 分类原文
- 阈值配置支持实时调整并生效
- 统计页展示各意图分布与准确率

---

## 八、测试策略
### 8.1 单元测试
- 规则匹配引擎单测（关键词/正则各 12 类）
- LLM 分类 prompt 构建单测
- 置信度阈值判定单测

### 8.2 集成测试
- 规则命中→直接输出全链路
- 规则未命中→LLM 分类全链路
- 紧急意图触发人工接管验证

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
