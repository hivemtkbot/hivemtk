# 线索管理 (Clue Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `clue-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 线索管理 |
| 功能名称（英文） | Clue Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | clue |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（clues / lead_scores / clue_activities）
- [x] 后端 Service 与 Controller
- [x] 前端页面（列表/详情/导入/统计）
- [x] 多渠道线索收集（表单/外部导入）
- [x] 智能评分（来源/行为/互动/时间）
- [x] 等级判定（Hot/Warm/Cold）
- [x] 自动培育触发
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

多渠道（抖音/小红书/表单/API/电话）汇集潜在客户信息，统一管理、评分、培育、转化。

### 2.2 解决思路

- ETL 清洗：去重（手机/邮箱）、格式校验、合并
- 智能评分：四维权重（来源 20% + 行为 30% + 互动 25% + 时间衰减 25%）
- 等级判定：Hot (≥80) / Warm (50-80) / Cold (<50)
- 自动培育：按等级触发不同营销流程

### 2.3 关键算法或模型

- 评分模型：`score = 0.2*source + 0.3*behavior + 0.25*interaction + 0.25*recency`
- 时间衰减：`recency = exp(-days_since_active / 30)`
- 去重策略：手机号优先 → 邮箱 → 微信 OpenID

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 姓名 |
| 输入 | phone | string | 是 | 手机号 |
| 输入 | email | string | 否 | 邮箱 |
| 输入 | source | string | 是 | 来源渠道 |
| 输入 | remark | text | 否 | 备注 |
| 输入 | tags | []string | 否 | 标签 |
| 输出 | clue_id | int64 | 是 | 线索ID |
| 输出 | score | int | 是 | 评分 |
| 输出 | level | string | 是 | Hot/Warm/Cold |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/clue | 线索列表 |
| POST | /api/clue | 创建线索 |
| GET | /api/clue/:id | 详情 |
| PUT | /api/clue/:id | 更新 |
| DELETE | /api/clue/:id | 删除 |
| POST | /api/clue/import | 批量导入 |
| GET | /api/clue/stats | 统计 |
| POST | /api/clue/:id/convert | 转化为客户 |
| POST | /api/clue/:id/nurture | 触发培育 |

### 3.3 安全与合规

- 手机号脱敏
- GDPR/个保法合规（同意授权）
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 列表查询 | < 300ms | ~150ms |
| 评分计算 | < 100ms | ~30ms |
| 批量导入 | 1000/批 | 1500/批 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/clue.go` | 接口 |
| Service | `internal/service/clue_service.go` | 业务 |
| Repository | `internal/repository/clue_repo.go` | 数据 |
| Model | `internal/model/clue.go` | 模型 |
| Infra | `internal/cron/` + Redis | 定时评分+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| customer-360 | 转化为客户 |
| 营销自动化 | 培育触发 |
| platform-lead | 平台端同步 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点 |
| 报表 | 数据来源 |

### 4.4 数据流向

```text
[外部渠道] → ETL 清洗
   → 去重 → 校验 → 评分
   → 写 clues + lead_scores
   → 等级判定
   → 触发培育（自动）或通知跟进（人工）
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 多渠道线索自动入库
2. 查看线索列表
3. 手动调整评分
4. 转化/培育/淘汰
5. 查看统计

### 5.2 系统处理流程

1. 接收线索
2. 去重
3. 计算初始评分
4. 写库
5. 异步重新评分（每日）
6. 等级变化 → 触发培育

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 重复线索 | 409001 | 合并更新 |
| 手机号无效 | 400101 | 拒绝 |

---

## 六、数据库设计

### 6.1 核心表 clues

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(64) | | 姓名 |
| phone | varchar(20) | | 手机号 |
| email | varchar(128) | | 邮箱 |
| wechat | varchar(64) | | 微信 |
| source | varchar(32) | 非空 | 来源 |
| score | int | 默认 0 | 评分 |
| level | varchar(16) | | Hot/Warm/Cold |
| status | varchar(16) | | new/contacting/converted/lost |
| tags | jsonb | | 标签 |
| last_active_at | timestamp | | 最后活跃 |
| created_at | timestamp | 非空 | |

### 6.2 核心表 lead_scores

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| clue_id | bigint | FK | 线索 |
| source_score | int | | 来源分 |
| behavior_score | int | | 行为分 |
| interaction_score | int | | 互动分 |
| recency_score | int | | 时间分 |
| total_score | int | 非空 | 总分 |
| level | varchar(16) | | 等级 |
| calculated_at | timestamp | 非空 | 计算时间 |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_clue_phone | phone | btree | 去重 |
| idx_clue_score | score | btree | 评分排序 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建线索 | 完整参数 | clue_id + 评分 | ✅ |
| TC-002 | 去重合并 | 重复手机号 | 合并更新 | ✅ |
| TC-003 | 自动评分 | 行为数据 | 评分更新 | ✅ |
| TC-004 | 等级触发培育 | Hot 线索 | 触发培育 | ✅ |
| TC-005 | 转化客户 | 选定线索 | 客户 ID | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SCORE_RECALC_CRON | SCORE_RECALC_CRON | 0 2 * * * |
| MAX_CLUES_PER_MERCHANT | MAX_CLUES_PER_MERCHANT | 100000 |

---

## 九、参考资料

- [customer-360.md](customer-360.md)
- [platform-lead.md](platform-lead.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
