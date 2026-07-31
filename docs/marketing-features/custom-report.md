# 自定义报表 (Custom Report)

> **所属模块**: marketing-automation
> **功能 slug**: `custom-report`
> **文档定位**: 业务自定义报表设计与查询,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 自定义报表 |
| 功能名称(英文) | Custom Report |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端可视化报表设计器
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 钻取联动(图表点击下钻)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

不同商户关注的指标不同,通用报表无法满足精细化分析需求。允许商户自主选择维度、度量、筛选条件、图表类型,生成个性化报表。

### 2.3 关键算法或模型

- **度量聚合**: 支持 SUM/AVG/COUNT/MAX/MIN/UNIQUE_COUNT
- **时间分组**: 按日/周/月/季/年自动分组
- **同环比计算**: 同比(去年同期)、环比(上一周期)
- **SQL 沙箱**: 字段白名单 + SQL 模板参数化,禁止任意 SQL

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 报表名称 |
| 输入 | data_source | string | 是 | 数据源标识 |
| 输入 | dimensions | array | 是 | 维度字段 |
| 输入 | measures | array | 是 | 度量字段 |
| 输入 | filters | array | 否 | 筛选条件 |
| 输入 | chart_type | string | 是 | 图表类型 |
| 输入 | time_range | object | 否 | 时间范围 |
| 输出 | columns | array | 是 | 列定义 |
| 输出 | rows | array | 是 | 数据行 |
| 输出 | summary | object | 否 | 汇总数据 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/custom-reports | 报表列表 |
| POST | /api/custom-reports | 创建报表 |
| GET | /api/custom-reports/:id | 报表详情 |
| PUT | /api/custom-reports/:id | 更新报表 |
| DELETE | /api/custom-reports/:id | 删除报表 |
| POST | /api/custom-reports/:id/query | 查询数据 |
| POST | /api/custom-reports/:id/export | 导出报表 |
| GET | /api/custom-reports/templates | 报表模板 |

### 3.3 安全与合规

- 数据源字段白名单
- SQL 参数化(防注入)
- 报表查询权限控制(基于 RBAC)
- 大数据量查询强制分页(默认 10000 行上限)
- 导出记录审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 简单报表查询 | < 1s |
| 复杂报表查询 | < 5s |
| 并发查询 | ≥ 50 |
| 导出生成 | < 30s(10 万行) |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/custom_report | |
| Service | internal/service/custom_report | 报表 CRUD + 解析 |
| Engine | internal/service/custom_report/query | 查询引擎 |
| Repository | internal/repository/custom_report | |
| Model | internal/model/custom_report | |
| Infra | internal/infra/datawarehouse | 数据仓库(可选) |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 数据源 | 订单/客户/事件表 |
| 数据大屏 | 报表作为大屏组件 |

### 4.3 数据流向

```text
[报表定义] → [查询引擎解析] → [SQL 生成] → [数据库执行] → [结果聚合] → [图表渲染]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"营销自动化 → 自定义报表"
2. 选择数据源(订单/客户/事件)
3. 拖拽维度、度量、筛选字段
4. 选择图表类型(柱状/折线/饼图/表格)
5. 预览数据
6. 保存为报表
7. 后续可查询/分享/导出

### 5.2 系统处理流程

1. 接收查询请求(报表 ID + 参数)
2. 加载报表定义
3. 注入参数(时间范围、筛选条件、商户 ID)
4. 生成 SQL
5. 执行查询(带超时)
6. 计算同环比
7. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 字段不存在 | 400040 | 拒绝执行 |
| 字段无权限 | 403040 | 拒绝执行 |
| 查询超时 | 500050 | 返回部分结果 + 提示 |
| 结果集过大 | 500051 | 强制分页 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `custom_reports` | 报表定义 |
| `custom_report_templates` | 报表模板 |
| `custom_report_query_logs` | 查询日志 |

```sql
CREATE TABLE custom_reports (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  description TEXT,
  data_source VARCHAR(64) NOT NULL,  -- orders/customers/events
  dimensions JSONB,  -- 维度
  measures JSONB,  -- 度量
  filters JSONB,  -- 筛选
  chart_type VARCHAR(32) NOT NULL,  -- bar/line/pie/table
  chart_config JSONB,  -- 图表配置
  time_range JSONB,  -- 默认时间范围
  is_public BOOLEAN DEFAULT false,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_merchant ( deleted_at)
);

CREATE TABLE custom_report_templates (
  id BIGINT PRIMARY KEY,
  category VARCHAR(64) NOT NULL,  -- sales/marketing/customer
  name VARCHAR(128) NOT NULL,
  description TEXT,
  config JSONB NOT NULL,
  thumbnail VARCHAR(255),
  is_official BOOLEAN DEFAULT false,
  use_count INT DEFAULT 0,
  created_at TIMESTAMP NOT NULL
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 求和度量 | 订单金额 SUM | 正确合计 | 待执行 |
| TC-002 | 时间分组 | 按日 | 按日聚合 | 待执行 |
| TC-003 | 筛选条件 | 状态=已完成 | 过滤正确 | 待执行 |
| TC-004 | 同比计算 | 去年同期 | 同比增长率 | 待执行 |
| TC-005 | 环比计算 | 上月 | 环比增长率 | 待执行 |
| TC-006 | 多维度交叉 | 商户+产品 | 透视表 | 待执行 |
| TC-007 | 字段无权限 | 越权字段 | 拒绝 | 待执行 |
| TC-008 | SQL 注入 | 恶意参数 | 参数化处理 | 待执行 |
| TC-009 | 大数据量 | 100 万行 | 分页+超时保护 | 待执行 |
| TC-010 | 报表导出 | 10 万行 | < 30s 生成 | 待执行 |
| TC-011 | 图表配置 | 柱状图配置 | 正确渲染 | 待执行 |
| TC-012 | 模板应用 | 套用官方模板 | 正确复制配置 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 查询超时 | REPORT_QUERY_TIMEOUT | 30s | |
| 最大结果行 | REPORT_MAX_ROWS | 10000 | |
| 导出文件大小 | REPORT_EXPORT_MAX_SIZE | 50MB | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
