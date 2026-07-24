# 闲鱼卡片 (Xianyu Card)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `card-xianyu`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 闲鱼卡片生成 |
| 功能名称（英文） | Xianyu Marketing Card |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | card |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构
- [x] 后端 Service 与 Controller
- [x] 前端页面（4 页）
- [x] Canvas 渲染（800×800 商品图比例）
- [x] 短链 + 活动追踪
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

闲鱼是二手交易平台，商品图标准为 800×800 方形，对卡片设计简洁、突出商品主题有较高要求。

### 2.2 解决思路

- 模板尺寸 800×800
- 优先展示商品图和价格
- 复用短链 + 活动追踪

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | template_id | string | 是 | 模板ID |
| 输入 | product_image | file/string | 是 | 商品图 |
| 输入 | price | decimal | 是 | 价格 |
| 输入 | title | string | 是 | 商品标题 |
| 输出 | card_id | int64 | 是 | 卡片ID |
| 输出 | image_url | string | 是 | 生成图URL |
| 输出 | short_url | string | 是 | 短链 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/xianyu-card/list | 列表 |
| POST | /api/xianyu-card/create | 创建 |
| GET | /api/xianyu-card/:id | 详情 |
| PUT | /api/xianyu-card/:id | 更新 |
| DELETE | /api/xianyu-card/:id | 删除 |
| GET | /api/xianyu-card/:id/stats | 统计 |
| POST | /api/xianyu-card/track/:code | 活动追踪 |

### 3.3 安全与合规

- 仅商户可创建
- 短链限流
- 内容审核

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 卡片渲染 | < 2s | ~1.0s |
| 短链跳转 | < 100ms | ~50ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/xianyu_card.go` | 卡片接口 |
| Service | `internal/service/xianyu_card_service.go` | 业务 |
| Repository | `internal/repository/xianyu_card_repo.go` | 数据 |
| Model | `internal/model/xianyu_card.go` | 模型 |
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
| auto-reply-xianyu | 闲鱼自动回复引用卡片 |

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择模板
2. 上传商品图
3. 填写标题和价格
4. 生成卡片

### 5.2 系统处理流程

1. 鉴权 + 配额
2. 图片压缩
3. Canvas 渲染
4. 内容审核
5. OBS 上传
6. 短链生成
7. 写库

---

## 六、数据库设计

### 6.1 核心表 xianyu_cards

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| template_id | varchar(32) | 非空 | 模板ID |
| title | varchar(128) | 非空 | 标题 |
| product_image | varchar(512) | 非空 | 商品图 |
| price | decimal(10,2) | 非空 | 价格 |
| image_url | varchar(512) | 非空 | 生成图URL |
| short_code | varchar(16) | UNIQUE | 短码 |
| status | tinyint | 非空 | 状态 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |
| deleted_at | timestamp | | 软删除 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 正常创建 | 完整参数 | card_id + image_url | ✅ |
| TC-002 | 活动追踪 | /track/:code | +1 view | ✅ |
| TC-003 | 价格格式错误 | 负数价格 | 400101 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| CARD_MAX_SIZE_MB | CARD_MAX_SIZE_MB | 5 |

---

## 九、参考资料

- [card-douyin.md](card-douyin.md)
- [auto-reply-xianyu.md](auto-reply-xianyu.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
