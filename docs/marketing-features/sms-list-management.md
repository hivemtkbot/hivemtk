# 短信列表与发送 (SMS List Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `sms-list-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 短信列表与发送 |
| 功能名称（英文） | SMS List & Send |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | sms |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（sms_lists / sms_subscribers / sms_send_logs）
- [x] 后端 Service 与 Controller
- [x] 列表管理 + 单条发送
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

短信列表 = 手机号集合 + 发送模板。提供列表管理 + 单条/批量发送能力。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | phone | string | 是 | 手机号 |
| 输入 | template_code | string | 是 | 模板 Code |
| 输入 | variables | jsonb | 否 | 模板变量 |
| 输出 | send_id | string | 是 | 发送流水号 |
| 输出 | status | string | 是 | success/failed |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/sms-list | 列表 |
| POST | /api/sms-list | 创建列表 |
| DELETE | /api/sms-list/:id | 删除列表 |
| GET | /api/sms-list/:id/subscribers | 收件人列表 |
| POST | /api/sms-list/:id/import | 批量导入手机号 |
| POST | /api/sms-list/send-single | 单条发送 |
| POST | /api/sms-list/send-batch | 批量发送 |
| POST | /api/sms-list/:id/resend | 重发 |

### 3.3 安全与合规

- 手机号格式校验
- 频次限制（同一号码 1 条/分钟）
- 内容审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 单条发送 | < 2s | ~1.2s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/sms_list.go` | 接口 |
| Service | `internal/service/sms_list_service.go` | 业务 |
| Repository | `internal/repository/sms_list_repo.go` | 数据 |
| Model | `internal/model/sms_list.go` | 模型 |
| Infra | `internal/sms/provider/` | Provider 适配器 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| sms-config | 服务商配置 |
| sms-jobs | 批量任务 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[商户] → 单条发送
   → [sms_list_service.SendSingle]
   → 调 SMS Provider（阿里云/腾讯云/华为云）
   → 写 sms_send_logs
   → 返回 send_id
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建列表
2. 导入手机号
3. 单条发送 / 重发 / 批量发送
4. 查看发送日志

### 5.2 系统处理流程

1. 鉴权
2. 调 Provider
3. 写日志
4. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 手机号无效 | 400101 | 拒绝 |
| 频次超限 | 429001 | 拒绝并提示 |

---

## 六、数据库设计

### 6.1 核心表 sms_lists / sms_subscribers / sms_send_logs

| 表 | 关键字段 | 说明 |
|---|---|---|
| sms_lists | name, subscriber_count | 列表 |
| sms_subscribers | list_id, phone, status | 收件人 |
| sms_send_logs | phone, template_code, status, error_msg | 发送日志 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 单条发送 | 真实手机号 | 200 + 收到 | ✅ |
| TC-002 | 重发 | send_id | 重新发送 | ✅ |
| TC-003 | 频次限制 | 同号 1 分钟内 | 429001 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SMS_PHONE_INTERVAL | SMS_PHONE_INTERVAL | 60 |

---

## 九、参考资料

- sms-config.md
- [sms-jobs-management.md](sms-jobs-management.md)
