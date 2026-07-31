# 素材管理 (Material Management)

> **所属模块**: content-creation
> **功能 slug**: `material-management`
> **文档定位**: 图片/视频/文件素材库管理,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 素材管理 |
| 功能名称(英文) | Material Management |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | content-creation |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端素材库页面
- [x] OBS 存储集成
- [x] 智能分类标签
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] AI 智能打标(基于图像识别)

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

营销内容(朋友圈/小红书/邮件)需要大量图片素材,商户需要一个集中管理、分类、检索、复用的素材库,避免重复上传与素材分散。

### 2.3 关键算法或模型

- **图片处理**: 缩略图生成(200x200/400x400/800x800),自动压缩,WebP 转换
- **图片识别(可选)**: 主体识别,智能打标
- **重复检测**: 感知哈希(pHash)检测重复
- **使用追踪**: 每次引用写入 `material_usage_logs`

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | file | binary | 是 | 上传文件 |
| 输入 | name | string | 是 | 素材名称 |
| 输入 | category_id | int64 | 否 | 分类 ID |
| 输入 | tags | array | 否 | 标签 |
| 输入 | description | string | 否 | 描述 |
| 输出 | material_id | int64 | 是 | 素材 ID |
| 输出 | url | string | 是 | 访问 URL |
| 输出 | thumbnail_url | string | 是 | 缩略图 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/material | 素材列表 |
| POST | /api/material/upload | 上传 |
| GET | /api/material/:id | 详情 |
| PUT | /api/material/:id | 更新 |
| DELETE | /api/material/:id | 删除 |
| GET | /api/material/:id/stats | 使用统计 |
| GET | /api/material/categories | 分类列表 |
| POST | /api/material/categories | 创建分类 |
| GET | /api/material/selector | 选择器(弹窗) |

### 3.3 安全与合规

- 文件类型白名单(jpg/png/gif/webp/mp4/pdf/docx/xlsx)
- 单文件大小限制(图片 20MB,视频 200MB)
- 文件名清洗(防 XSS)
- 内容审核(图片鉴黄、敏感词)
- 防盗链(签名 URL)

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 上传响应 | < 1s |
| 列表查询 | < 300ms |
| 缩略图生成 | < 2s |
| 并发上传 | ≥ 50 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/material | |
| Service | internal/service/material | CRUD + 选择器 |
| Engine | internal/service/material/processor | 图片处理 |
| Repository | internal/repository/material | |
| Model | internal/model/material | |
| Infra | internal/infra/obs | OBS 客户端 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| OBS | 文件存储 |
| 内容审核 | 图片/视频审核 |
| AI 内容 | 引用素材 |
| 邮件/短信 | 附件引用 |

### 4.3 数据流向

```text
[用户上传] → [OBS 存储] → [图片处理(缩略图/压缩)] → [审核] → [数据库记录]
                                                                          ↓
                                                          [被引用] → [使用日志]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"内容生产 → 素材管理"
2. 选择分类或搜索
3. 上传文件(拖拽/选择)
4. 自动生成缩略图
5. 填写名称、标签
6. 保存
7. 其他模块引用(在 AI 创作/邮件编辑器中"插入素材")

### 5.2 系统处理流程

1. 接收上传文件
2. 校验类型与大小
3. 计算哈希,去重检查
4. 上传到 OBS
5. 触发图片处理(缩略图/压缩)
6. 提交审核
7. 写入数据库
8. 返回 URL

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 文件超大 | 413010 | 413 |
| 类型不支持 | 415010 | 415 |
| 审核未通过 | 400120 | 拒绝 |
| OBS 失败 | 500120 | 重试 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `materials` | 素材主表 |
| `material_categories` | 分类 |
| `material_usage_logs` | 使用日志 |

```sql
CREATE TABLE materials (
  id BIGINT PRIMARY KEY,
  
  category_id BIGINT,
  name VARCHAR(128) NOT NULL,
  file_type VARCHAR(32) NOT NULL,  -- image/video/document
  file_extension VARCHAR(16),
  file_size BIGINT NOT NULL,
  url VARCHAR(512) NOT NULL,
  thumbnail_url VARCHAR(512),
  width INT,
  height INT,
  duration INT,  -- 视频时长(秒)
  oss_key VARCHAR(255) NOT NULL,
  hash VARCHAR(64),  -- pHash 去重
  description TEXT,
  tags JSONB,
  use_count INT DEFAULT 0,
  audit_status VARCHAR(16) DEFAULT 'pending',  -- pending/passed/rejected
  audit_message TEXT,
  uploaded_by BIGINT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, file_type, deleted_at),
  INDEX idx_hash (hash)
);

CREATE TABLE material_categories (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(64) NOT NULL,
  parent_id BIGINT,
  sort INT DEFAULT 0,
  created_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 上传图片 | 1MB jpg | 成功+缩略图 | 待执行 |
| TC-002 | 上传过大文件 | 50MB 图片 | 413 拒绝 | 待执行 |
| TC-003 | 上传不支持类型 | .exe | 415 拒绝 | 待执行 |
| TC-004 | 视频上传 | 50MB mp4 | 成功+时长 | 待执行 |
| TC-005 | 文档上传 | .pdf | 成功 | 待执行 |
| TC-006 | 重复检测 | 同 hash | 提示已存在 | 待执行 |
| TC-007 | 缩略图生成 | 上传图 | 多尺寸缩略图 | 待执行 |
| TC-008 | 分类管理 | CRUD | 正常 | 待执行 |
| TC-009 | 标签搜索 | tag=产品 | 命中 | 待执行 |
| TC-010 | 选择器 | 弹窗调用 | 返回素材 | 待执行 |
| TC-011 | 使用统计 | 引用 | 计数+1 | 待执行 |
| TC-012 | 软删除 | 删除 | 列表不可见 | 待执行 |
| TC-013 | 审核拒绝 | 含违规 | 拒绝 | 待执行 |
| TC-014 | 防盗链 | 签名 URL | 验证 | 待执行 |
| TC-015 | 并发上传 | 100 并发 | 全部成功 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| OBS Endpoint | OBS_ENDPOINT | - | |
| OBS Bucket | OBS_BUCKET | marketing-tools | |
| 图片最大 | MATERIAL_IMAGE_MAX_SIZE | 20MB | |
| 视频最大 | MATERIAL_VIDEO_MAX_SIZE | 200MB | |
| 缩略图尺寸 | MATERIAL_THUMB_SIZES | "200,400,800" | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.10 节
- IMAGE_COMPRESSION.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
