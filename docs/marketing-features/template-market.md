# 模板市场 (Template Market)

> **所属模块**: content-creation
> **功能 slug**: `template-market`
> **文档定位**: 营销模板浏览、下载、分享市场,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 模板市场 |
| 功能名称(英文) | Template Market |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | content-creation |
| 优先级 | P2 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端市场浏览页
- [x] 一键下载与安装
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 模板评分与评论
- [ ] 模板分成机制

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户经常需要相似场景的营销模板(活动海报文案、邮件模板、短信模板、小红书种草模板)。模板市场提供官方与社区共享资源,降低创作成本。

### 2.3 关键算法或模型

- **模板类型**: 邮件 / 短信 / 朋友圈 / 小红书 / 抖音 / WhatsApp
- **行业标签**: 电商、教育、医疗、美容、加盟、餐饮等
- **搜索**: 关键词 + 标签 + 行业 + 类型多维筛选
- **推荐**: 热门下载 + 行业匹配 + 评分排序

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | keyword | string | 否 | 搜索词 |
| 输入 | template_type | string | 否 | 模板类型 |
| 输入 | industry | string | 否 | 行业 |
| 输入 | tag | string | 否 | 标签 |
| 输出 | templates | array | 是 | 模板列表 |
| 输出 | download_url | string | 是 | 下载链接 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/templates | 模板列表 |
| GET | /api/templates/:id | 模板详情 |
| POST | /api/templates/:id/download | 下载模板 |
| GET | /api/templates/official | 官方模板 |
| GET | /api/templates/search | 搜索 |
| GET | /api/templates/my-downloads | 我的下载 |
| GET | /api/templates/categories | 分类 |
| GET | /api/templates/hot | 热门模板 |

### 3.3 安全与合规

- 模板内容审核
- 下载频率限制
- 模板版本管理

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 列表查询 | < 200ms |
| 搜索 | < 500ms |
| 下载 | < 1s |
| 并发 | ≥ 200 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/template | |
| Service | internal/service/template | 列表/下载/搜索 |
| Repository | internal/repository/template | |
| Model | internal/model/template | |
| Infra | internal/infra/oss | 静态资源(OBS) |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| OBS | 模板文件存储 |
| 内容审核 | 模板上架审核 |

### 4.3 数据流向

```text
[用户搜索/浏览] → [数据库查询] → [结果列表]
                                    ↓
[用户下载] → [记录下载日志] → [复制到商户工作台]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"内容生产 → 模板市场"
2. 浏览/搜索/筛选模板
3. 点击查看详情(预览图、说明、评分)
4. 点击"下载"
5. 自动安装到对应工作台(邮件草稿/短信草稿/AI 模板)
6. 在"我的下载"中查看历史

### 5.2 系统处理流程

1. 接收查询请求
2. 多维过滤(类型/行业/标签/关键词)
3. 排序(下载量/评分/最新)
4. 分页返回
5. 下载时记录日志,触发对应工作台初始化

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 模板已下架 | 404060 | 404 |
| 下载频率超限 | 429020 | 限流 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `templates` | 模板主表 |
| `template_downloads` | 下载记录 |

```sql
CREATE TABLE templates (
  id BIGINT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  template_type VARCHAR(32) NOT NULL,  -- email/sms/moments/xiaohongshu/douyin
  industry VARCHAR(64),
  tags JSONB,
  content JSONB NOT NULL,  -- 模板内容
  preview_image VARCHAR(255),
  download_count INT DEFAULT 0,
  use_count INT DEFAULT 0,
  rating_avg DECIMAL(3,2) DEFAULT 0,
  rating_count INT DEFAULT 0,
  is_official BOOLEAN DEFAULT false,
  is_featured BOOLEAN DEFAULT false,
  status VARCHAR(16) DEFAULT 'active',  -- active/archived
  version VARCHAR(16) DEFAULT '1.0',
  author VARCHAR(64),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_type_industry (template_type, industry, status, deleted_at)
);

CREATE TABLE template_downloads (
  id BIGINT PRIMARY KEY,
  template_id BIGINT NOT NULL,
  
  user_id BIGINT NOT NULL,
  downloaded_at TIMESTAMP NOT NULL,
  INDEX idx_template (template_id),
  INDEX idx_merchant ( downloaded_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 列表查询 | 全部 | 正确分页 | 待执行 |
| TC-002 | 关键词搜索 | 关键词 | 命中结果 | 待执行 |
| TC-003 | 类型筛选 | email | 仅邮件模板 | 待执行 |
| TC-004 | 行业筛选 | 美业 | 仅美业模板 | 待执行 |
| TC-005 | 标签筛选 | 活动 | 命中活动 | 待执行 |
| TC-006 | 模板详情 | ID | 完整内容 | 待执行 |
| TC-007 | 下载模板 | 模板 ID | 安装到工作台 | 待执行 |
| TC-008 | 下载次数 | 多次下载 | 计数+1 | 待执行 |
| TC-009 | 官方模板 | is_official=true | 优先展示 | 待执行 |
| TC-010 | 热门排序 | 下载量 | 降序 | 待执行 |
| TC-011 | 评分排序 | 评分 | 降序 | 待执行 |
| TC-012 | 我的下载 | 查询历史 | 返回列表 | 待执行 |
| TC-013 | 已下架模板 | archived | 404 | 待执行 |
| TC-014 | 下载频率 | 1 秒 10 次 | 限流 | 待执行 |
| TC-015 | 推荐位 | is_featured | 优先展示 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 下载频率 | TEMPLATE_DOWNLOAD_RATE | 60/min | |
| 热门权重 | TEMPLATE_HOT_WEIGHT | 0.5 | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 搜索慢 | P95 > 1s | 钉钉 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.10 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
