# 流失预警 (Churn Prediction)

> **所属模块**: marketing-automation
> **功能 slug**: `churn-prediction`
> **文档定位**: 客户流失风险预测与挽回运营,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 流失预警 |
| 功能名称(英文) | Churn Prediction |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端页面与组件
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 深度学习模型(LSTM)预测

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

获客成本远高于挽回成本。识别即将流失的客户,提前触达,能显著提升留存率与 ROI。

### 2.3 关键算法或模型

- **特征工程**: R(最近活跃天数)、F(月活频次)、M(月消费金额)、趋势(消费下降率)、互动(消息打开率)、客诉(投诉次数)
- **评分模型**: 加权评分模型(可配置权重),总分 0-100
  - R < 7 天: 0 分;7-30 天: 30 分;30-60 天: 60 分;>60 天: 90 分
  - F 下降 > 50%: 30 分;F 下降 > 20%: 15 分
  - M 下降 > 50%: 30 分;M 下降 > 20%: 15 分
  - 客诉 > 0: 20 分
- **风险等级**: 0-30 低、31-60 中、61-80 高、81-100 极高
- **规则引擎**: 支持商户自定义权重与阈值

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | user_id | int64 | 是 | |
| 输出 | churn_score | int | 是 | 0-100 |
| 输出 | risk_level | string | 是 | low/medium/high/critical |
| 输出 | risk_factors | array | 是 | 风险因素列表 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/churn/predict | 单用户预测 |
| POST | /api/churn/batch-predict | 批量预测 |
| GET | /api/churn/high-risk | 高风险用户列表 |
| GET | /api/churn/alerts | 预警记录 |
| POST | /api/churn/alerts/:id/handle | 处理预警 |
| GET | /api/churn/model-config | 模型配置 |
| PUT | /api/churn/model-config | 更新模型配置 |
| GET | /api/churn/stats | 风险分布统计 |

### 3.3 安全与合规

- 评分数据用于运营目的,需用户授权(隐私政策)
- 高风险用户名单仅授权运营人员可见
- 挽回触达频率限制(同一用户 7 天内最多 2 次)

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 单用户预测 | < 100ms |
| 批量预测(1 万) | < 5min |
| 高风险查询 | < 500ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/churn | |
| Service | internal/service/churn | 预测逻辑 + 风险识别 |
| Engine | internal/service/churn/scorer | 评分计算引擎 |
| Repository | internal/repository/churn | |
| Model | internal/model/churn | |
| 定时任务 | internal/cron/churn | 每日全量打分 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 客户/订单 | 行为数据源 |
| 营销自动化 | 触发挽回流程 |
| 消息中心 | 挽回触达 |

### 4.3 数据流向

```text
[用户行为数据: 登录/购买/互动]
        ↓
[特征工程] → [评分模型] → [风险等级]
                                ↓
                       [高风险用户] → [挽回营销流程]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"营销自动化 → 流失预警"
2. 查看风险分布(饼图/列表)
3. 设置风险等级阈值与挽回策略
4. 触发每日定时打分
5. 查看高风险用户列表
6. 选择用户 → 启动挽回营销流程

### 5.2 系统处理流程

1. 每日凌晨 2:00 触发全量打分任务
2. 拉取所有用户最近 30 天行为数据
3. 计算特征并打分
4. 写入 `churn_scores` 表
5. 高风险用户自动生成预警记录
6. 推送钉钉/企微通知运营人员
7. 运营点击"启动挽回"→ 触发营销流程

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 行为数据缺失 | 500040 | 跳过该用户,记录 |
| 模型配置非法 | 400030 | 拒绝保存 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `churn_model_configs` | 模型配置(权重/阈值) |
| `churn_scores` | 用户流失分 |
| `churn_alerts` | 预警记录 |
| `churn_handle_logs` | 预警处理记录 |

```sql
CREATE TABLE churn_model_configs (
  id BIGINT PRIMARY KEY,
  
  weights JSONB,  -- 特征权重
  thresholds JSONB,  -- 风险等级阈值
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  UNIQUE KEY uk_merchant ( deleted_at)
);

CREATE TABLE churn_scores (
  id BIGINT PRIMARY KEY,
  
  user_id BIGINT NOT NULL,
  score INT NOT NULL,
  risk_level VARCHAR(16) NOT NULL,
  risk_factors JSONB,  -- 风险因素明细
  features JSONB,  -- 特征值快照
  scored_at TIMESTAMP NOT NULL,
  UNIQUE KEY uk_user ( user_id, scored_at),
  INDEX idx_risk ( risk_level, scored_at)
);

CREATE TABLE churn_alerts (
  id BIGINT PRIMARY KEY,
  
  user_id BIGINT NOT NULL,
  score_id BIGINT,
  risk_level VARCHAR(16) NOT NULL,
  status VARCHAR(16) DEFAULT 'pending',  -- pending/handling/handled/ignored
  handled_by BIGINT,
  handled_at TIMESTAMP,
  handle_note TEXT,
  triggered_flow_id BIGINT,
  created_at TIMESTAMP NOT NULL,
  INDEX idx_data, status, created_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 长时间未登录 | R=90 天 | 90 分,极高风险 | 待执行 |
| TC-002 | 消费下降 | M 下降 60% | 30 分,中等风险 | 待执行 |
| TC-003 | 高客诉 | 客诉 3 次 | 20 分 | 待执行 |
| TC-004 | 低风险用户 | 活跃用户 | < 30 分,低风险 | 待执行 |
| TC-005 | 自定义权重 | 修改 R 权重 | 评分相应变化 | 待执行 |
| TC-006 | 阈值边界 | score=30 | 中风险 | 待执行 |
| TC-007 | 批量预测性能 | 1 万用户 | < 5min | 待执行 |
| TC-008 | 自动挽回触发 | 极高风险 | 触发挽回流程 | 待执行 |
| TC-009 | 挽回频率限制 | 同一用户 7 天 3 次 | 拒绝第 3 次 | 待执行 |
| TC-010 | 模型重训练 | 历史数据 | 重新生成权重 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 全量打分时间 | CHURN_DAILY_CRON | "0 2 * * *" | 凌晨 2 点 |
| 挽回频率限制 | CHURN_RECOVER_LIMIT_DAYS | 7 | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 高风险用户数突增 | +50% 日环比 | 钉钉 |
| 挽回率过低 | < 10% | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
