# 标签分层 (Tag Segmentation)

> **所属模块**: cdp
> **功能 slug**: `tagSegmentation`
> **文档定位**: 客户标签体系管理，支持手动 + 自动打标，驱动精细化分层运营。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 标签分层 |
| 功能名称(英文) | Tag Segmentation |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | cdp |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 客户标签体系（行业/兴趣/行为/价值/生命周期 5 大类）
- [x] 手动打标与批量打标
- [x] 自动打标规则引擎（行为事件触发）
- [x] `internal/controller/user_segment.go` 后端控制器
- [x] 标签分组与客户数统计
- [x] 前端 `user-web/src/views/tagSegmentation/List.vue` 标签管理
- [x] 标签与营销流程、客户分群联动

### 1.2 待完成内容
- [ ] 标签智能推荐（基于客户画像自动建议标签）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
私域运营需要对客户进行精细化分层，按行业、兴趣、行为、价值、生命周期等维度打标，从而支撑精准营销、智能体话术个性化、客户分群触达等场景。统一的标签体系是 CDP 的核心基础。

### 2.2 解决思路
建立分类标签体系（5 大类多级标签），支持手动打标与基于行为事件的自动打标规则；标签变更通过事件总线广播，下游模块（营销流程、客户分群、智能体）订阅消费；标签客户数实时统计。

### 2.3 关键算法或模型
- 自动打标规则引擎：事件条件 + 标签动作（增/删/覆盖）
- 标签客户数统计：基于标签索引表实时计数 + 增量更新
- 标签去重：标签规范化（小写、去空格、同义词归并）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 标签名称 |
| 输入 | category | string | 是 | 标签分类 |
| 输入 | rule_config | object | 否 | 自动打标规则 |
| 输入 | auto_apply | bool | 否 | 是否自动应用 |
| 输出 | tag_id | int64 | 是 | 标签 ID |
| 输出 | customer_count | int64 | 是 | 关联客户数 |
| 输出 | auto_apply | bool | 是 | 是否自动应用 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 标签列表查询 < 300ms
- 自动打标规则触发延迟 < 2s
- 标签客户数统计准确率 100%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/tag-segmentation/list | 标签列表 | JWT |
| POST | /api/tag-segmentation | 新建标签 | JWT |
| PUT | /api/tag-segmentation/:id | 更新标签 | JWT |
| DELETE | /api/tag-segmentation/:id | 删除标签 | JWT |
| POST | /api/tag-segmentation/:id/apply | 手动打标（批量客户） | JWT |
| GET | /api/tag-segmentation/:id/customers | 标签下客户列表 | JWT |
| POST | /api/tag-segmentation/rule-test | 自动打标规则测试 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| tags | 标签主表 |
| customer_tags | 客户-标签关联表 |
| tag_rules | 自动打标规则表 |
| tag_categories | 标签分类表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| tag_id | bigint | 标签 ID |
| name | varchar(64) | 标签名称 |
| category | varchar(32) | 分类（industry/interest/behavior/value/lifecycle） |
| rule_config | jsonb | 自动打标规则配置 |
| customer_count | bigint | 关联客户数 |
| auto_apply | bool | 是否自动应用 |

---

## 六、业务流程
### 6.1 主流程
1. 运营人员在标签管理页新建标签，选择分类与规则
2. 手动打标：选择客户批量打标
3. 自动打标：客户行为事件触发规则引擎，命中则自动打标
4. 标签变更通过事件总线广播，下游模块订阅消费
5. 标签客户数实时统计与展示

### 6.2 异常处理
- 规则引擎异常：记录失败日志，跳过该事件，告警
- 标签删除：解除所有客户关联，提示下游引用
- 标签重复：归并到主标签

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 标签分层管理 | /tag-segmentation | tagSegmentation/List.vue |

### 7.2 关键交互
- 左侧标签分类树（5 大类）
- 右侧标签列表，展示客户数与自动应用状态
- 标签新建/编辑表单（含规则配置器）
- 手动打标弹窗（客户多选 + 标签选择）
- 规则测试工具（输入事件查看打标结果）

---

## 八、测试策略
### 8.1 单元测试
- 自动打标规则引擎单测（多事件组合条件）
- 标签归并与去重单测
- 客户数统计单测

### 8.2 集成测试
- 标签变更事件总线广播测试
- 标签与营销流程、客户分群联动测试
- 大批量打标性能测试（万级客户）

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
