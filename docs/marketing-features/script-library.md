# 话术库 (Script Library)

> **所属模块**: content-creation
> **功能 slug**: `script-library`
> **文档定位**: 智能体话术模板与推荐,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 话术库 |
| 功能名称(英文) | Script Library |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | content-creation |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端话术管理页
- [x] 智能推荐(基于场景/客户标签)
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] AI 自动挖掘优秀话术(从对话数据)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

智能体在跟进客户时需要快速调用经过验证的应对话术(开场、异议处理、促单),系统化的话术库能提升新人出单率与整体转化。

### 2.3 关键算法或模型

- **标签匹配**: 基于客户标签/行业/阶段匹配候选话术
- **效果评分**: 综合采纳率(0-1)、转化率(0-1)、评分(1-5)加权
- **使用频率衰减**: 近期使用越多排序越前
- **推荐算法**: 标签匹配 40% + 效果评分 30% + 时效 20% + 个性化 10%

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 话术名称 |
| 输入 | category | string | 是 | 场景分类 |
| 输入 | industry | string | 否 | 行业 |
| 输入 | content | text | 是 | 话术内容(支持变量) |
| 输入 | variables | array | 否 | 变量定义 |
| 输入 | tags | array | 否 | 标签 |
| 输入 | scenario | string | 否 | 使用场景描述 |
| 输出 | script_id | int64 | 是 | 话术 ID |
| 输出 | recommended_scripts | array | 是 | 推荐列表 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/scripts | 话术列表 |
| POST | /api/scripts | 创建话术 |
| GET | /api/scripts/:id | 话术详情 |
| PUT | /api/scripts/:id | 更新话术 |
| DELETE | /api/scripts/:id | 删除话术 |
| GET | /api/scripts/categories | 分类列表 |
| POST | /api/scripts/recommend | 智能推荐 |
| POST | /api/scripts/:id/use | 记录使用 |
| POST | /api/scripts/:id/rate | 评分 |
| GET | /api/scripts/search | 全文搜索 |

### 3.3 安全与合规

- 话术内容审核(违规词检测)
- 私域话术需符合平台规范(微信/抖音等)
- 用户操作审计

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 列表查询 | < 200ms |
| 智能推荐 | < 500ms |
| 全文搜索 | < 1s |
| 并发查询 | ≥ 100 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/script | |
| Service | internal/service/script | CRUD + 推荐 |
| Engine | internal/service/script/recommender | 推荐引擎 |
| Repository | internal/repository/script | |
| Model | internal/model/script | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 客户标签 | 推荐标签匹配 |
| AI 内容 | 智能润色 |
| 异议处理模板 | 关联引用 |

### 4.3 数据流向

```text
[当前客户标签/行业/阶段] → [匹配候选] → [评分排序] → [Top N 推荐]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"内容生产 → 话术库"
2. 按分类浏览(开场/跟进/异议/促单)
3. 搜索关键词
4. 点击"查看详情"→ 完整内容
5. 一键复制或带变量渲染
6. 使用后评分反馈

### 5.2 系统处理流程(推荐)

1. 接收推荐请求(客户 ID + 场景)
2. 加载客户标签与历史
3. 查询匹配话术(分类 + 行业 + 标签)
4. 计算综合得分
5. 返回 Top 5

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 话术不存在 | 404050 | 404 |
| 违规词命中 | 400110 | 拒绝保存 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `scripts` | 话术主表 |
| `script_categories` | 分类字典 |
| `script_use_logs` | 使用日志 |
| `script_ratings` | 评分 |

```sql
CREATE TABLE scripts (
  id BIGINT PRIMARY KEY,
  
  category VARCHAR(32) NOT NULL,  -- opening/followup/objection/closing/aftersales
  name VARCHAR(128) NOT NULL,
  content TEXT NOT NULL,
  variables JSONB,  -- 变量定义 [{name, default_value, description}]
  industry VARCHAR(64),
  tags JSONB,  -- 标签数组
  scenario TEXT,  -- 使用场景
  use_count INT DEFAULT 0,
  rating_avg DECIMAL(3,2) DEFAULT 0,
  rating_count INT DEFAULT 0,
  conversion_rate DECIMAL(5,4) DEFAULT 0,  -- 转化率
  is_official BOOLEAN DEFAULT false,  -- 官方话术
  status VARCHAR(16) DEFAULT 'active',
  created_by BIGINT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, category, deleted_at),
  INDEX idx_tags ( tags) USING GIN
);
```

### 6.2 索引

- `( category, deleted_at)` — 分类查询
- `tags GIN 索引` — 标签查询

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建话术 | 完整字段 | 200 OK | 待执行 |
| TC-002 | 变量插值 | {{name}} | 正确替换 | 待执行 |
| TC-003 | 分类筛选 | category=opening | 仅开场话术 | 待执行 |
| TC-004 | 全文搜索 | 关键词 | 命中结果 | 待执行 |
| TC-005 | 标签推荐 | 客户标签匹配 | Top 5 | 待执行 |
| TC-006 | 推荐评分 | 高效果话术 | 排序靠前 | 待执行 |
| TC-007 | 使用统计 | 多次使用 | 计数正确 | 待执行 |
| TC-008 | 评分影响 | 5 星 | 评分上升 | 待执行 |
| TC-009 | 软删除 | 删除 | 列表不再显示 | 待执行 |
| TC-010 | 官方话术 | 平台创建 | 标记 official | 待执行 |
| TC-011 | 跨实例隔离 | 商户 A 话术 | 商户 B 不可见 | 待执行 |
| TC-012 | 违规词检测 | 含敏感词 | 拒绝保存 | 待执行 |
| TC-013 | 批量导入 | 100 条 | 全部成功 | 待执行 |
| TC-014 | 推荐性能 | 1000 话术 | < 500ms | 待执行 |
| TC-015 | 推荐个性化 | 用户历史采纳 | 个性化排序 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 推荐数量 | SCRIPT_RECOMMEND_TOP | 5 | |
| 推荐缓存 | SCRIPT_RECOMMEND_CACHE | 5min | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 推荐响应慢 | P95 > 1s | 钉钉 |
| 话术命中率低 | < 30% | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.10 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
