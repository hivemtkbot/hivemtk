# 商户初始化向导 (Merchant Initialization)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `merchant-initialization`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

> **📌 边界说明**: 本文档聚焦**新商户首次入驻的多步骤配置流程**(企业信息 → 联系人 → 功能模块 → 默认配置 → 完成确认)。
> - 平台对**商户主体**的 CRUD / 审批 / 统计见 [platform-merchant.md](../../hivemtk-platform/docs/platform-features/platform-merchant.md)
> - 商户与平台之间的**通信接口**(`/merchant-api`)见 [merchant-api.md](../../hivemtk-platform/docs/platform-features/merchant-api.md)
> - 注意:初始化流程生成商户标识 `merchant_key` + API Key,开源版无 License 流程

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 商户初始化向导 |
| 功能名称（英文） | Merchant Initialization Wizard |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | merchant-init |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（merchants / merchant_configs / init_logs）
- [x] 后端 Service 与 Controller
- [x] 前端页面（MerchantInit.vue，多步骤表单）
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

新商户注册后，必须完成企业信息、行业类型、功能模块选择等初始化流程，才能正式使用系统。初始化数据将作为后续所有业务的基础配置。

### 2.3 关键算法或模型

- 营业执照 OCR 识别（可选）
- 商户ID 生成：MD5(phone + timestamp) 截取前 12 位
- API Key 生成：UUID 去除短横线

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | company_name | string | 是 | 公司名 |
| 输入 | industry | string | 是 | 行业类型 |
| 输入 | scale | string | 否 | 规模 |
| 输入 | business_license | string | 否 | 营业执照号 |
| 输入 | contact_name | string | 是 | 联系人 |
| 输入 | contact_phone | string | 是 | 联系手机 |
| 输入 | enabled_modules | []string | 是 | 启用的功能模块 |
| 输出 | merchant_key | string | 是 | 商户标识(唯一) |
| 输出 | api_key | string | 是 | API 密钥 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/merchant/init/step1 | 保存企业信息 |
| POST | /api/merchant/init/step2 | 保存联系人信息 |
| POST | /api/merchant/init/step3 | 保存功能模块 |
| POST | /api/merchant/init/step4 | 保存默认配置 |
| POST | /api/merchant/init/complete | 完成初始化 |
| GET | /api/merchant/init/draft | 获取草稿 |
| GET | /api/merchant/init/status | 获取初始化状态 |

### 3.3 安全与合规

- 每步保存都需要登录态
- 草稿自动加密存储
- 完成时生成唯一商户ID
- API Key 仅返回一次，丢失需重新生成

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 步骤保存响应 | < 300ms | ~150ms |
| 并发初始化 | 100+ | 200+ |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/merchant.go` | 初始化接口 |
| Service | `internal/service/merchant_init_service.go` | 业务编排 |
| Repository | `internal/repository/merchant_repo.go` | 商户数据 |
| Model | `internal/model/merchant.go` | 商户模型 |
| Infra | `internal/cache/redis.go` | 草稿缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 登录态校验 |
| obs-config | 对象存储配置 |
| email-smtp | 默认 SMTP |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 所有业务模块 | 校验 merchant_key 是否激活（开源版：JWT 鉴权 + install.lock 标记初始化完成） |

### 4.4 数据流向

```text
MerchantInit.vue (5 步表单)
  → POST /api/merchant/init/step{1-5}
  → [merchant.go] → [merchant_init_service]
  → [merchant_repo] → [PostgreSQL]
  → 草稿写入 Redis（24h TTL）
  → 完成时生成 merchant_key + api_key
  → 写入 merchants 表
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 注册成功自动跳转到初始化页
2. 依次填写 5 个步骤表单
3. 每步自动保存草稿
4. 完成后展示商户ID + API Key
5. 引导进入系统主界面

### 5.2 系统处理流程

1. 校验当前用户是否已初始化（避免重复）
2. 步骤参数校验
3. 业务校验（手机号格式、必填项）
4. 保存草稿到 Redis
5. 步骤5 完成时持久化到 PostgreSQL
6. 生成 merchant_key + api_key
7. 失效草稿
8. 写初始化日志

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 已初始化 | 409001 | 跳转首页 |
| 草稿过期 | 410001 | 提示重新填写 |
| 营业执照号格式错误 | 400101 | 提示格式 |
| API Key 重复 | 500001 | 重试 |

---

## 六、数据库设计

### 6.1 核心表 merchants

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| api_key | varchar(64) | UNIQUE | API 密钥 |
| company_name | varchar(128) | 非空 | 公司名 |
| industry | varchar(32) | | 行业 |
| scale | varchar(16) | | 规模 |
| business_license | varchar(64) | | 营业执照号 |
| contact_name | varchar(32) | | 联系人 |
| contact_phone | varchar(20) | | 联系手机 |
| enabled_modules | jsonb | | 启用模块 |
| status | tinyint | 非空 | 0=初始化中 1=正常 2=禁用 |
| created_at | timestamp | 非空 | 创建时间 |
| updated_at | timestamp | 非空 | 更新时间 |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_data

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 完整初始化 | 5步完整数据 | merchant_key + api_key | ✅ |
| TC-002 | 断点续填 | 步骤2退出 | 步骤3加载步骤1-2数据 | ✅ |
| TC-003 | 重复初始化 | 已完成用户 | 409001 | ✅ |
| TC-004 | API Key 唯一性 | 并发生成 | 全部唯一 | ✅ |
| TC-005 | 草稿过期 | 等待 24h | 410001 | ✅ |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| INIT_DRAFT_TTL | INIT_DRAFT_TTL | 86400 |
| API_KEY_LENGTH | API_KEY_LENGTH | 32 |

---

## 九、参考资料

- [ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- [platform-merchant.md](../../hivemtk-platform/docs/platform-features/platform-merchant.md)
