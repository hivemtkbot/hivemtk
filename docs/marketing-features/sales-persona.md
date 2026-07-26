# 销冠画像 (Sales Persona)

> **所属模块**: ai-agent-core
> **功能 slug**: `persona`
> **文档定位**: 销冠能力画像建模，用于智能体配置参考与团队培训。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 销冠画像 |
| 功能名称(英文) | Sales Persona |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | ai-agent-core |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] 销冠能力画像模型（话术风格/异议处理/成交节奏/客户偏好）
- [x] `internal/controller/sales_persona_controller.go` 后端控制器
- [x] 画像 CRUD 与标签化管理
- [x] 画像与智能体配置绑定
- [x] 前端 `user-web/src/views/persona/List.vue` 画像管理
- [x] 画像效果对比报表

### 1.2 待完成内容
- [ ] 基于真实销冠会话自动生成画像

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
不同销冠的销售风格、话术节奏、异议处理策略各有特色。将这些能力结构化为画像，可作为 AI 销售智能体的配置模板，让智能体复用销冠打法；同时用于团队培训，让新人快速学习销冠方法论。

### 2.3 关键算法或模型
- 能力维度抽取：基于 LLM 的会话摘要 + 关键短语提取
- 风格聚类：话术风格标签向量化 + K-Means 聚类
- 画像匹配：智能体配置时按画像标签与目标客户匹配度排序

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 画像名称 |
| 输入 | style_tags | array | 是 | 话术风格标签 |
| 输入 | talk_patterns | array | 是 | 话术模式 |
| 输入 | objection_strategies | array | 是 | 异议处理策略 |
| 输入 | closing_techniques | array | 是 | 成交技巧 |
| 输入 | customer_preferences | array | 否 | 客户偏好 |
| 输出 | persona_id | int64 | 是 | 画像 ID |
| 输出 | effectiveness_score | decimal | 是 | 效果评分 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 画像列表查询 < 300ms
- 画像绑定智能体生效延迟 < 5s
- 画像维度覆盖率 ≥ 90%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/persona/list | 画像列表 | JWT |
| GET | /api/persona/:id | 画像详情 | JWT |
| POST | /api/persona | 新建画像 | JWT |
| PUT | /api/persona/:id | 更新画像 | JWT |
| DELETE | /api/persona/:id | 删除画像 | JWT |
| POST | /api/persona/:id/bind-agent | 绑定智能体 | JWT |
| GET | /api/persona/:id/effectiveness | 画像效果报表 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| sales_personas | 销冠画像主表 |
| persona_agent_bindings | 画像与智能体绑定关系表 |
| persona_effectiveness_logs | 画像效果日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| persona_id | bigint | 画像 ID |
| name | varchar(64) | 画像名称 |
| style_tags | jsonb | 话术风格标签数组 |
| talk_patterns | jsonb | 话术模式数组 |
| objection_strategies | jsonb | 异议处理策略数组 |
| closing_techniques | jsonb | 成交技巧数组 |
| customer_preferences | jsonb | 客户偏好数组 |

---

## 六、业务流程
### 6.1 主流程
1. 运营人员在画像管理页新建画像，填写各维度能力标签
2. 画像保存后可绑定到一个或多个智能体
3. 智能体运行时读取绑定画像，将画像参数注入 LLM Prompt
4. 画像效果报表按绑定的智能体成交转化率统计
5. 支持画像复制、对比、归档

### 6.2 异常处理
- 画像被引用时删除：拒绝并提示绑定关系
- 画像维度为空：校验必填维度，提示补全
- 智能体绑定多个画像：以最后绑定的画像为准并告警

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 销冠画像管理 | /persona | persona/List.vue |

### 7.2 关键交互
- 画像卡片列表，展示风格标签与效果评分
- 画像编辑抽屉（多维度标签编辑器）
- 智能体绑定弹窗（多选）
- 画像效果对比图表（雷达图）
- 画像复制为模板按钮

---

## 八、测试策略
### 8.1 单元测试
- 画像 CRUD 单测
- 画像维度校验单测
- 绑定关系管理单测

### 8.2 集成测试
- 画像绑定智能体后运行时注入测试
- 画像效果报表数据准确性测试
- 多画像并发绑定冲突测试
