# TikTok 卡片 (TikTok Card)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `card-tiktok`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | TikTok 卡片生成 |
| 功能名称（英文） | TikTok Marketing Card |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | card |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构
- [x] 后端 Service 与 Controller
- [x] 前端页面（4 页）
- [x] Canvas 渲染（1080×1920 竖版）
- [x] 多语言支持（英/日/韩/泰等）
- [x] 短链 + 活动追踪
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

TikTok 海外版，覆盖东南亚/欧美/中东/日韩等市场，卡片需支持多语言文案和文化适配。

### 2.2 解决思路

- 模板尺寸 1080×1920
- 多语言 i18n 字段
- 短链区分地区（短码包含地区前缀）

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | template_id | string | 是 | 模板ID |
| 输入 | lang | string | 否 | 语言（en/ja/ko/th等） |
| 输入 | title | string | 是 | 标题 |
| 输入 | content | string | 否 | 内容 |
| 输出 | card_id | int64 | 是 | 卡片ID |
| 输出 | image_url | string | 是 | 生成图URL |
| 输出 | short_url | string | 是 | 短链 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/tiktok-card/list | 列表 |
| POST | /api/tiktok-card/create | 创建 |
| GET | /api/tiktok-card/:id | 详情 |
| PUT | /api/tiktok-card/:id | 更新 |
| DELETE | /api/tiktok-card/:id | 删除 |
| GET | /api/tiktok-card/:id/stats | 统计 |
| POST | /api/tiktok-card/track/:code | 活动追踪 |

### 3.3 安全与合规

- 仅商户可创建
- 短链限流
- 内容审核（含文化敏感性）
- 软删除

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 卡片渲染 | < 2s | ~1.4s |
| 短链跳转 | < 100ms | ~50ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/tiktok_card.go` | 卡片接口 |
| Service | `internal/service/tiktok_card_service.go` | 业务 |
| Repository | `internal/repository/tiktok_card_repo.go` | 数据 |
| Model | `internal/model/tiktok_card.go` | 模型 |
| Infra | Canvas + OBS + Redis | 渲染+存储+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| short-link | 短链 |
| obs-config | 图片存储 |
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| auto-reply-tiktok | TikTok 自动回复引用卡片 |

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择地区 + 语言
2. 选择模板
3. 填写多语言文案
4. 生成卡片

### 5.2 系统处理流程

1. 鉴权 + 配额
2. i18n 字段校验
3. Canvas 渲染（含字体回退）
4. 内容审核
5. OBS 上传
6. 短链生成（含地区前缀）
7. 写库

---

## 六、数据库设计

### 6.1 核心表 tiktok_cards

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| template_id | varchar(32) | 非空 | 模板ID |
| region | varchar(8) | 非空 | 地区 |
| lang | varchar(8) | 非空 | 语言 |
| title | varchar(256) | 非空 | 标题（i18n） |
| content | text | | 内容（i18n） |
| image_url | varchar(512) | 非空 | 生成图URL |
| short_code | varchar(16) | UNIQUE | 短码 |
| status | tinyint | 非空 | 状态 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |
| deleted_at | timestamp | | 软删除 |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_ttcard_region | region | btree | 地区维度 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 多语言创建 | ja 文案 | 卡片正确渲染日文 | ✅ |
| TC-002 | 活动追踪 | /track/:code | +1 view | ✅ |
| TC-003 | 地区短码 | US 地区 | 短码含 us 前缀 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SUPPORTED_LANGS | SUPPORTED_LANGS | en,ja,ko,th,id,vi |

---

## 九、参考资料

- [FUNCTION_DETAILS.md](../architecture/FUNCTION_DETAILS.md#二营销模块---卡片管理)
- [card-douyin.md](card-douyin.md)
- [auto-reply-tiktok.md](auto-reply-tiktok.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
