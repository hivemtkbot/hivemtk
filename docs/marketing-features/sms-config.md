# 短信配置 (SMS Config)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `sms-config`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 短信服务商配置 |
| 功能名称（英文） | SMS Provider Config |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | sms |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（sms_configs）
- [x] 后端 Service 与 Controller
- [x] 多服务商支持（阿里云/腾讯云/华为云）
- [x] 凭据加密存储
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

国内短信发送需对接云服务商 SDK。支持多服务商配置，灵活切换。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | provider | string | 是 | aliyun/tencent/huawei |
| 输入 | access_key | string | 是 | 访问 Key |
| 输入 | access_secret | string | 是 | 访问 Secret |
| 输入 | sign_name | string | 是 | 短信签名 |
| 输入 | template_code | string | 是 | 模板 Code |
| 输出 | config_id | int64 | 是 | 配置ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/sms-config | 配置列表 |
| POST | /api/sms-config | 创建配置 |
| GET | /api/sms-config/:id | 详情 |
| PUT | /api/sms-config/:id | 更新 |
| DELETE | /api/sms-config/:id | 删除 |
| POST | /api/sms-config/:id/test | 发送测试短信 |

### 3.3 安全与合规

- 凭据加密存储
- 签名/模板报备验证
- 发送频率限制（防刷）
- 短信内容审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 发送延迟 | < 2s | ~1.2s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/sms_config.go` | 接口 |
| Service | `internal/service/sms_config_service.go` | 业务 |
| Repository | `internal/repository/sms_config_repo.go` | 数据 |
| Model | `internal/model/sms_config.go` | 模型 |
| Infra | `internal/sms/provider/*.go` | 服务商适配器 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| utils/secrets | 凭据加密 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| sms-jobs | 任务使用配置 |

### 4.4 数据流向

```text
[商户] → 选择服务商 → 填写凭据
   → [sms_config_service.Create]
   → 加密 access_secret → 写库
   → 测试发送（可选）
   → 返回 config_id
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择服务商（阿里云/腾讯云/华为云）
2. 填写 AccessKey / Secret
3. 填写短信签名（已报备）
4. 填写模板 Code（已报备）
5. 测试发送

### 5.2 系统处理流程

1. 鉴权
2. 加密 Secret
3. 写库
4. 异步测试发送
5. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 凭据无效 | 401001 | 提示检查 |
| 模板未报备 | 400101 | 提示报备 |

---

## 六、数据库设计

### 6.1 核心表 sms_configs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| provider | varchar(32) | 非空 | 服务商 |
| access_key | varchar(128) | 非空 | 访问 Key |
| access_secret_enc | varchar(512) | 非空 | 加密 Secret |
| sign_name | varchar(32) | 非空 | 签名 |
| template_code | varchar(64) | 非空 | 模板 Code |
| daily_limit | int | 默认 1000 | 日发送上限 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建配置 | 阿里云 | config_id | ✅ |
| TC-002 | 测试发送 | 真实号码 | 200 + 收到短信 | ✅ |
| TC-003 | 加密验证 | 读库 | 密文存储 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SMS_RATE_LIMIT | SMS_RATE_LIMIT | 100 |
| SMS_DAILY_LIMIT | SMS_DAILY_LIMIT | 1000 |

---

## 九、参考资料

- [sms-jobs-management.md](sms-jobs-management.md)
- SENSITIVE_DATA_ENCRYPTION.md
