# OneID 身份统一 (OneID)

> **所属模块**: cdp
> **功能 slug**: `oneid`
> **文档定位**: 客户多渠道身份归一化合并为统一客户档案，冲突时人工裁决。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | OneID 身份统一 |
| 功能名称(英文) | OneID |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | cdp |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 多渠道身份归一化算法（手机号 → 微信 union_id → 邮箱 → 设备指纹）
- [x] 统一客户档案合并
- [x] 冲突检测与 `oneid_conflicts` 表
- [x] 冲突人工裁决工作台
- [x] `internal/controller/customer_oneid_controller.go` + 路由 `/oneid/list`、`/oneid/conflicts`
- [x] 前端 OneID 列表与冲突裁决页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] OneID 合并历史追溯
- [ ] 自动裁决策略（高置信度冲突自动合并）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
客户在多渠道有不同身份（手机/微信/邮箱/企微 external_userid），导致同一客户被识别为多人。OneID 通过归一化算法合并为统一客户档案，冲突时进入人工裁决流程。

### 2.3 关键算法或模型
- 标识符优先级匹配算法
- 冲突检测（多标识符交叉匹配不一致）
- 置信度计算（命中标识符数量与优先级加权）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | source_ids | array | 是 | 渠道身份标识符列表 |
| 输入 | match_type | string | 是 | phone/wechat/email/device |
| 输入 | confidence | float | 否 | 置信度 |
| 输出 | one_id | int64 | 是 | 统一客户 ID |
| 输出 | resolved_status | string | 是 | resolved/conflict/pending |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 归一化匹配 < 50ms
- 单客户标识符上限 20
- 冲突检测准确率 ≥ 95%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/oneid/list | OneID 列表 | JWT |
| GET | /api/oneid/:id | OneID 详情 | JWT |
| POST | /api/oneid/resolve | 归一化解析（内部） | JWT |
| GET | /api/oneid/conflicts | 冲突列表 | JWT |
| POST | /api/oneid/conflicts/:id/resolve | 冲突裁决 | JWT |
| POST | /api/oneid/merge | 手动合并 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| customer_oneids | 统一客户档案 |
| customer_oneid_identities | 渠道身份标识符 |
| oneid_conflicts | 冲突记录 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| one_id | bigint | 统一客户 ID |
| source_ids | jsonb | 渠道身份标识符列表 |
| match_type | varchar(16) | phone/wechat/email/device |
| confidence | float | 置信度 |
| resolved_status | varchar(16) | resolved/conflict/pending |

---

## 六、业务流程
### 6.1 主流程
1. 渠道接入新客户身份
2. OneID 归一化：按优先级匹配标识符
3. 命中已有 OneID → 合并标识符到该 OneID
4. 未命中 → 创建新 OneID
5. 多标识符冲突 → 写入 `oneid_conflicts` 表
6. 人工在工作台裁决（合并 / 拆分 / 忽略）
7. 裁决后更新 OneID 与标识符关系

### 6.2 异常处理
- 标识符格式无效：跳过该标识符，记录日志
- 冲突长期未裁决：定期提醒人工处理
- 合并后反悔：支持拆分操作（保留历史）

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| OneID 列表 | /oneid/list | oneid/List.vue |
| 冲突裁决 | /oneid/conflicts | oneid/Conflicts.vue |

### 7.2 关键交互
- 列表按 match_type、resolved_status 筛选
- OneID 详情展示所有渠道标识符与合并历史
- 冲突裁决页对比冲突双方信息
- 合并/拆分操作带二次确认

---

## 八、测试策略
### 8.1 单元测试
- 归一化优先级匹配单测
- 冲突检测单测
- 置信度计算单测

### 8.2 集成测试
- 新身份→归一化→合并到已有 OneID 全链路
- 冲突→写入 conflicts 表→人工裁决全链路
- 拆分操作还原验证
