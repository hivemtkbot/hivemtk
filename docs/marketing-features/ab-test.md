# A/B 测试 (A/B Test)

> **所属模块**: marketing-automation
> **功能 slug**: `ab-test`
> **文档定位**: 营销活动 A/B 实验管理与转化分析,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | A/B 测试 |
| 功能名称(英文) | A/B Test |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端页面与组件
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 多变量实验(MVT)支持

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户在开展营销活动时,需要对比不同文案、不同落地页、不同发送时间的转化效果,以数据驱动决策。

### 2.2 解决思路

将用户流量按哈希随机分流到多个实验组(默认 50/50,可调),统计每组的转化率、置信度,自动判定胜出组。

### 2.3 关键算法或模型

- **流量分桶**: `bucket(user_id) = hash(user_id + experiment_id) % 100`
- **转化率计算**: `rate = conversions / exposures`
- **统计显著性**: 双比例 Z 检验, p < 0.05 视为显著
- **胜出判定**: 当样本量 > 最小样本数且 p < 0.05 时,自动判定胜出组

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 实验名称 |
| 输入 | variants | array | 是 | 实验变体 [{name, weight, content}] |
| 输入 | traffic_allocation | int | 否 | 流量比例(0-100) |
| 输入 | min_sample_size | int | 否 | 最小样本数 |
| 输出 | experiment_id | int64 | 是 | 实验 ID |
| 输出 | assigned_variant | string | 是 | 用户分配的变体 |
| 输出 | winner | string | 否 | 胜出变体 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/ab-experiments | 实验列表 |
| POST | /api/ab-experiments | 创建实验 |
| GET | /api/ab-experiments/:id | 实验详情 |
| POST | /api/ab-experiments/:id/start | 开始实验 |
| POST | /api/ab-experiments/:id/pause | 暂停实验 |
| POST | /api/ab-experiments/:id/stop | 停止实验 |
| GET | /api/ab-experiments/:id/results | 实验结果 |
| POST | /api/ab-experiments/:id/track | 转化事件上报 |

### 3.3 安全与合规

- 实验哈希分桶基于 user_id + experiment_id,独立部署下每个实例独立测试
- 用户身份归一(OneID)
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 分桶响应 | < 10ms (P99) |
| 转化事件上报 | < 50ms (P95) |
| 结果统计查询 | < 500ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/ab_experiment | |
| Service | internal/service/ab_experiment | 实验管理 + 分桶 |
| Engine | internal/service/ab_experiment/stats | 统计与显著性检验 |
| Repository | internal/repository/ab_experiment | |
| Model | internal/model/ab_experiment | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 客户/线索 | 用户身份 |
| 事件追踪 | 转化事件源 |
| 报表 | 统计可视化 |

### 4.3 数据流向

```text
[用户触发活动] → [分桶引擎: hash → variant] → [展示变体内容]
                                            ↓
                                       [转化事件] → [统计] → [显著性检验] → [胜出判定]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"A/B 测试 → 实验管理"
2. 点击"新建实验",填写基本信息
3. 配置 2-N 个变体(每个变体:名称、流量权重、内容)
4. 设置转化目标与最小样本数
5. 启动实验
6. 实验运行中查看实时数据
7. 达到显著条件后,系统提示胜出组
8. 停止实验并全量应用胜出变体

### 5.2 系统处理流程

1. 用户访问活动页面
2. 查询活动关联的有效实验
3. 对 user_id 哈希分桶,返回对应变体内容
4. 记录曝光事件
5. 用户完成转化行为时,上报转化事件
6. 后台统计服务按分钟聚合曝光/转化
7. 当样本量满足条件时,计算显著性
8. 胜出组明确后,触发通知

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 流量比例总和 ≠ 100 | 400011 | 校验失败 |
| 实验变体为空 | 400012 | 校验失败 |
| 重复启动 | 400013 | 返回已运行状态 |
| 样本量不足 | - | 提示"数据收集中" |

### 5.4 状态机

```text
[草稿] → [运行中] → [已完成]
            ↓
         [已暂停] → [运行中]
            ↓
         [已停止]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `ab_experiments` | 实验定义主表 |
| `ab_variants` | 实验变体表 |
| `ab_assignments` | 用户分桶记录 |
| `ab_conversions` | 转化事件记录 |

```sql
CREATE TABLE ab_experiments (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  description TEXT,
  goal VARCHAR(64),  -- click/convert/purchase
  traffic_allocation INT DEFAULT 100,  -- 0-100
  min_sample_size INT DEFAULT 1000,
  status VARCHAR(16) DEFAULT 'draft',  -- draft/running/paused/stopped/completed
  winner_variant_id BIGINT,
  started_at TIMESTAMP,
  ended_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, status, deleted_at)
);

CREATE TABLE ab_variants (
  id BIGINT PRIMARY KEY,
  experiment_id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL,
  weight INT NOT NULL,  -- 流量权重
  content JSONB,  -- 变体内容
  exposures INT DEFAULT 0,
  conversions INT DEFAULT 0,
  conversion_rate DECIMAL(10,4) DEFAULT 0,
  INDEX idx_experiment (experiment_id)
);
```

### 6.2 索引

- `ab_assignments`: `( experiment_id, user_id)` UNIQUE
- `ab_conversions`: `( experiment_id, variant_id, created_at)`

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 哈希分桶一致性 | user_id+exp_id | 同用户始终同桶 | 待执行 |
| TC-002 | 流量比例准确 | 50/50, 1000 用户 | 各 500 左右 | 待执行 |
| TC-003 | 转化统计 | 100 曝光,20 转化 | 20% 转化率 | 待执行 |
| TC-004 | 显著性检验 | p<0.05 显著差异 | 标记胜出组 | 待执行 |
| TC-005 | 样本量不足 | < min_sample | 提示数据收集中 | 待执行 |
| TC-006 | 变体修改限制 | 运行中编辑 | 拒绝编辑 | 待执行 |
| TC-007 | 跨实验分桶独立性 | 两个实验同用户 | 互不影响 | 待执行 |
| TC-008 | 停用实验 | 实验停止 | 后续流量走默认 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 显著性阈值 | AB_TEST_P_VALUE_THRESHOLD | 0.05 | |
| 最大变体数 | AB_TEST_MAX_VARIANTS | 10 | |
| 统计聚合周期 | AB_TEST_AGGREGATE_INTERVAL | 1m | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 显著实验未及时通知 | > 1h | 钉钉 |
| 转化率异常波动 | > 50% 变化 | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
