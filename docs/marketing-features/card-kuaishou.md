# 快手卡片 (Kuaishou Card)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `card-kuaishou`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 快手卡片生成 |
| 功能名称（英文） | Kuaishou Marketing Card |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | card |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（kuaishou_cards / kuaishou_card_stats）
- [x] 后端 Service 与 Controller
- [x] 前端页面（List/Create/Detail/Stats 4 页）
- [x] API 接口与 Swagger 文档
- [x] Canvas 渲染引擎（1080×1920 竖版）
- [x] 短链 + 活动追踪（view/like/share/collect/comment）
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

快手平台竖版视频卡片制作，支持模板、背景图、标题、二维码、短链追踪。

### 2.2 解决思路

- 模板尺寸统一 1080×1920（竖版视频封面比例）
- 复用 Canvas 引擎，差异在模板列表
- 短链 + 活动追踪同抖音卡片

### 2.3 关键算法或模型

- Canvas 渲染
- 短码生成（同抖音）
- 活动追踪：Redis HASH + 落库

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | template_id | string | 是 | 模板ID |
| 输入 | title | string | 是 | 标题 |
| 输入 | subtitle | string | 否 | 副标题 |
| 输入 | qrcode_url | string | 否 | 二维码 |
| 输入 | bg_image | file/string | 否 | 背景图 |
| 输出 | card_id | int64 | 是 | 卡片ID |
| 输出 | image_url | string | 是 | 生成图URL |
| 输出 | short_url | string | 是 | 短链 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/kuaishou-card/list | 卡片列表 |
| POST | /api/kuaishou-card/create | 创建卡片 |
| GET | /api/kuaishou-card/:id | 卡片详情 |
| PUT | /api/kuaishou-card/:id | 更新卡片 |
| DELETE | /api/kuaishou-card/:id | 删除卡片 |
| GET | /api/kuaishou-card/:id/stats | 统计数据 |
| POST | /api/kuaishou-card/track/:code | 活动追踪 |

### 3.3 安全与合规

- 仅商户可创建自己卡片
- 短链访问限流
- 图片内容审核
- 软删除

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 卡片渲染 | < 2s | ~1.3s |
| 短链跳转 | < 100ms | ~50ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/kuaishou_card.go` | 卡片接口 |
| Service | `internal/service/kuaishou_card_service.go` | 渲染+短链业务 |
| Repository | `internal/repository/kuaishou_card_repo.go` | 数据访问 |
| Model | `internal/model/kuaishou_card.go` | 数据模型 |
| Infra | `internal/canvas/render.go` + OBS + Redis | 渲染+存储+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| short-link | 短链生成 |
| obs-config | 图片存储 |
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点可发送卡片 |

### 4.4 数据流向

```text
商户 → KuaishouCard Create → 模板 + 内容
   → Canvas 渲染 → OBS 上传 → 短链生成
   → 写库 → 返回结果
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入快手卡片 → 创建
2. 选择模板（竖版 1080×1920）
3. 填写内容
4. 实时预览
5. 生成卡片 → 复制短链

### 5.2 系统处理流程

1. 鉴权 + 配额
2. 参数校验
3. Canvas 渲染
4. 内容审核
5. 上传 OBS
6. 生成短链
7. 写库

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 模板不存在 | 404001 | 提示"模板已下架" |
| 图片超限 | 400101 | 提示大小限制 |
| 审核不通过 | 403001 | 提示"内容违规" |

---

## 六、数据库设计

### 6.1 核心表 kuaishou_cards

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| template_id | varchar(32) | 非空 | 模板ID |
| title | varchar(128) | 非空 | 标题 |
| subtitle | varchar(256) | | 副标题 |
| image_url | varchar(512) | 非空 | 生成图URL |
| short_code | varchar(16) | UNIQUE | 短码 |
| qrcode_url | varchar(512) | | 二维码 |
| status | tinyint | 非空 | 0=草稿 1=已发布 2=已下架 |
| created_at | timestamp | 非空 | 创建时间 |
| updated_at | timestamp | 非空 | 更新时间 |
| deleted_at | timestamp | | 软删除 |

### 6.2 核心表 kuaishou_card_stats

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| card_id | bigint | FK | 卡片ID |
| view_count | int | 默认 0 | 浏览数 |
| like_count | int | 默认 0 | 点赞数 |
| share_count | int | 默认 0 | 分享数 |
| collect_count | int | 默认 0 | 收藏数 |
| comment_count | int | 默认 0 | 评论数 |
| stat_date | date | 非空 | 统计日期 |

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 正常创建 | 完整参数 | card_id + image_url | ✅ |
| TC-002 | 活动追踪 | GET /track/:code | +1 view | ✅ |
| TC-003 | 软删除 | DELETE | 列表不可见 | ✅ |
| TC-004 | 并发追踪 | 100 并发 | 计数准确 | ✅ |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| CARD_MAX_SIZE_MB | CARD_MAX_SIZE_MB | 5 |

---

## 九、参考资料

- [card-douyin.md](card-douyin.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
