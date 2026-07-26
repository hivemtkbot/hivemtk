# 置信度/拟人度/反馈学习面板 (Tuning Panel)

> **所属模块**: ai-agent-core
> **功能 slug**: `tuning`
> **文档定位**: 三大 AI 调优配置统一管理面板（置信度阈值 + 拟人度评分规则 + 反馈学习数据集）。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 置信度/拟人度/反馈学习面板 |
| 功能名称(英文) | Tuning Panel |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] 意图识别置信度阈值配置
- [x] 拟人度评分规则配置（< 0.85 触发重生成）
- [x] 反馈学习数据集管理
- [x] `setupTuningRoutes` 路由注册
- [x] `internal/controller/tuning_controller.go` 后端控制器
- [x] 统一管理面板前端页面
- [x] 配置版本管理与回滚
- [x] 调优效果对比报表

### 1.2 待完成内容
- [ ] 自动化调参（基于 A/B 测试）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
AI 销售智能体的回复质量依赖三大调优配置：意图识别置信度阈值决定何时转人工、拟人度评分规则决定回复是否够"像人"、反馈学习数据集用于持续优化模型。这三类配置散落在不同模块，需要一个统一面板集中管理。

### 2.3 关键算法或模型
- 置信度阈值：意图识别 score < threshold 时转人工
- 拟人度评分：多维度评分（流畅度/相关性/语气/长度），加权汇总，< 0.85 触发重生成
- 反馈学习：基于人工标注的正负样本微调 LLM

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | config_type | string | 是 | 配置类型（confidence/humanity/feedback） |
| 输入 | threshold | float | 否 | 置信度阈值 |
| 输入 | scoring_rules | array | 否 | 拟人度评分规则 |
| 输入 | feedback_dataset_id | int64 | 否 | 反馈数据集 ID |
| 输出 | config_id | int64 | 是 | 配置 ID |
| 输出 | version | int | 是 | 版本号 |
| 输出 | effectiveness | object | 是 | 调优效果 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 配置生效延迟 < 5s
- 拟人度评分计算 < 200ms
- 反馈数据集查询 < 1s

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/tuning/configs | 配置列表 | JWT |
| GET | /api/tuning/configs/:type | 按类型查询配置 | JWT |
| PUT | /api/tuning/configs/:id | 更新配置 | JWT |
| POST | /api/tuning/configs/:id/publish | 发布配置（生成新版本） | JWT |
| POST | /api/tuning/configs/:id/rollback | 回滚到上一版本 | JWT |
| GET | /api/tuning/effectiveness | 调优效果对比报表 | JWT |
| GET | /api/tuning/feedback-datasets | 反馈数据集列表 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| tuning_configs | 调优配置主表 |
| tuning_config_versions | 配置版本历史表 |
| feedback_datasets | 反馈学习数据集表 |
| tuning_effectiveness_logs | 调优效果日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| config_type | varchar(32) | 配置类型（confidence/humanity/feedback） |
| threshold | decimal(4,3) | 置信度阈值 |
| scoring_rules | jsonb | 拟人度评分规则数组 |
| feedback_dataset_id | bigint | 反馈数据集 ID |
| version | int | 版本号 |

---

## 六、业务流程
### 6.1 主流程
1. 运营人员在调优面板编辑三类配置
2. 保存后生成草稿版本，可预览效果
3. 发布后配置生效，智能体运行时读取最新配置
4. 意图识别 score < threshold 时转人工
5. 拟人度评分 < 0.85 时触发回复重生成
6. 反馈数据集积累后触发 LLM 微调
7. 调优效果报表对比前后指标

### 6.2 异常处理
- 配置格式校验失败：拒绝保存，提示错误字段
- 发布失败：回滚到上一版本，告警
- 微调任务失败：保留旧模型，记录失败日志

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 调优管理面板 | /tuning | tuning/List.vue |

### 7.2 关键交互
- 三个 Tab（置信度/拟人度/反馈学习）
- 置信度阈值滑块与转人工预览
- 拟人度评分规则可视化编辑器（维度 + 权重）
- 反馈数据集列表与样本预览
- 配置版本时间轴与回滚按钮
- 调优效果对比图表（折线图）

---

## 八、测试策略
### 8.1 单元测试
- 置信度阈值判断单测
- 拟人度评分计算单测
- 配置版本管理单测

### 8.2 集成测试
- 配置发布生效端到端测试
- 拟人度评分 < 0.85 触发重生成测试
- 反馈数据集微调流程测试
