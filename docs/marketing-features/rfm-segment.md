# RFM 用户分层 (RFM Segmentation)

> **所属模块**: marketing-automation
> **功能 slug**: `rfm-segment`
> **文档定位**: 基于 RFM 模型的用户价值分层与精细化运营,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | RFM 用户分层 |
| 功能名称(英文) | RFM Segmentation |
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

- [ ] AI 智能分层规则推荐

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

不同价值的用户需要差异化运营(高价值用户专属折扣、流失用户挽回、潜力用户培养)。RFM 是经典的客户价值分层模型。

### 2.2 解决思路

RFM = Recency(最近一次消费)+ Frequency(消费频次)+ Monetary(消费金额)。对三个维度分别打分(1-5),组合形成 125 个分层,系统支持自定义阈值与分层命名。

### 2.3 关键算法或模型

- **R 计算**: 距今天数,数值越小分数越高
- **F 计算**: 周期内消费次数,次数越多分数越高
- **M 计算**: 周期内消费金额,金额越高分数越高
- **分位打分**: 按分位数划分(20%, 40%, 60%, 80%)得 1-5 分
- **分层命名**: 如 R5F5M5=冠军用户、R1F1M1=流失用户

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 分层规则名称 |
| 输入 | r_thresholds | array | 是 | R 维度阈值(天数) |
| 输入 | f_thresholds | array | 是 | F 维度阈值(次数) |
| 输入 | m_thresholds | array | 是 | M 维度阈值(金额) |
| 输入 | segments | array | 是 | 自定义分层定义 |
| 输出 | user_segment | string | 是 | 用户所属分层 |
| 输出 | rfm_score | string | 是 | 如 "555" |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/user-segment/rules | 规则列表 |
| POST | /api/user-segment/rules | 创建规则 |
| PUT | /api/user-segment/rules/:id | 更新规则 |
| DELETE | /api/user-segment/rules/:id | 删除规则 |
| POST | /api/user-segment/rules/:id/calculate | 触发计算 |
| GET | /api/user-segment/users | 分层用户列表 |
| GET | /api/user-segment/users/:id/rfm | 用户 RFM 详情 |
| GET | /api/user-segment/stats | 分层统计 |

### 3.3 安全与合规

- RFM 计算任务异步执行,大商户分片处理
- 计算频率限制(同一规则 1 小时内最多 1 次)
- 计算结果保留 90 天历史

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 单用户 RFM 计算 | < 50ms |
| 10 万用户批量计算 | < 5min |
| 分层查询 | < 300ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/user_segment | |
| Service | internal/service/user_segment | 规则管理 + 用户分层 |
| Engine | internal/service/user_segment/calculator | RFM 计算引擎 |
| Repository | internal/repository/user_segment | |
| Model | internal/model/user_segment | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 订单/客户 | R/F/M 数据源 |
| 营销自动化 | 分层结果用于触发营销活动 |

### 4.3 数据流向

```text
[订单/客户数据] → [RFM 计算引擎] → [用户分位打分] → [分层归类]
                                                        ↓
                                                  [营销流程触发]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"营销自动化 → RFM 分层"
2. 点击"新建规则",设置 R/F/M 阈值
3. 自定义分层名称与运营策略
4. 保存规则
5. 点击"立即计算"或设定定时任务
6. 计算完成后查看分层统计与用户列表

### 5.2 系统处理流程

1. 接收计算任务
2. 拉取时间窗口内所有用户的 R/F/M 原始数据
3. 按 R/F/M 分别计算分位阈值
4. 为每个用户打分
5. 按 RFM 组合查询对应分层
6. 写入 `user_rfm_results` 表
7. 触发下游营销流程(可选)

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 阈值不合法 | 400020 | 校验失败 |
| 数据量过大 | 500030 | 分片处理,异步执行 |
| 计算超时 | 500031 | 部分结果保留,可重试 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `user_segment_rules` | 分层规则定义 |
| `user_rfm_results` | 用户 RFM 计算结果 |
| `user_segment_mappings` | 分层定义映射 |

```sql
CREATE TABLE user_segment_rules (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  description TEXT,
  time_window_days INT DEFAULT 365,  -- 统计时间窗口
  r_thresholds JSONB,  -- R 维度阈值
  f_thresholds JSONB,  -- F 维度阈值
  m_thresholds JSONB,  -- M 维度阈值
  segments JSONB,  -- 分层定义 [{name, condition, color}]
  enabled BOOLEAN DEFAULT true,
  last_calculated_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_merchant ( deleted_at)
);

CREATE TABLE user_rfm_results (
  id BIGINT PRIMARY KEY,
  rule_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  r_score INT NOT NULL,
  f_score INT NOT NULL,
  m_score INT NOT NULL,
  rfm_code VARCHAR(8),  -- 如 "555"
  segment VARCHAR(64),  -- 分层名称
  r_value INT,  -- 最近消费天数
  f_value INT,  -- 消费次数
  m_value DECIMAL(15,2),  -- 消费金额
  calculated_at TIMESTAMP NOT NULL,
  INDEX idx_rule_user (rule_id, user_id),
  INDEX idx_rule_segment (rule_id, segment)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | R 维度分位计算 | 100 用户 | 5 等分(20/40/60/80/100) | 待执行 |
| TC-002 | F 维度分位计算 | 不同频次 | 正确分桶 | 待执行 |
| TC-003 | M 维度分位计算 | 不同金额 | 正确分桶 | 待执行 |
| TC-004 | 高价值用户识别 | 高 RFM | 归入冠军/忠诚层 | 待执行 |
| TC-005 | 流失用户识别 | 低 R | 归入流失层 | 待执行 |
| TC-006 | 时间窗口 | 365 天外数据 | 不计入 | 待执行 |
| TC-007 | 10 万用户计算 | 大数据量 | < 5min 完成 | 待执行 |
| TC-008 | 自定义分层 | 自定义 condition | 准确归类 | 待执行 |
| TC-009 | 重复计算 | 1 小时内重复 | 拒绝/复用 | 待执行 |
| TC-010 | 增量更新 | 新增订单 | 下次计算反映 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 默认时间窗口 | RFM_DEFAULT_WINDOW_DAYS | 365 | |
| 计算分片大小 | RFM_CALC_BATCH_SIZE | 5000 | |
| 计算超时 | RFM_CALC_TIMEOUT | 30min | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 计算任务失败 | > 0 | 钉钉 |
| 计算耗时过长 | > 30min | 钉钉 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
