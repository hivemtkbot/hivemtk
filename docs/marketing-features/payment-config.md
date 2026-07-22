# 支付配置 (Payment Config)

> **所属模块**: order-payment
> **功能 slug**: `payment-config`
> **文档定位**: 多支付渠道配置,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 支付配置 |
| 功能名称(英文) | Payment Config |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | order-payment |
| 优先级 | P1 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端配置管理页
- [x] 多渠道支持(微信/支付宝/Stripe)
- [x] 沙箱/生产环境切换
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 数字货币支付

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户需要对接多种支付渠道(微信支付、支付宝、Stripe、银联),配置各渠道的 AppID、商户号、密钥、回调地址,支持沙箱测试。

### 2.2 解决思路

每种支付渠道抽象为统一 `Provider` 接口,配置存储在 DB(加密),运行时注入到支付网关。

### 2.3 关键算法或模型

- **配置加密**: 密钥 AES-256-GCM
- **环境切换**: sandbox/production
- **Webhook 地址**: 平台签名校验
- **证书管理**: 微信/支付宝证书上传到 OBS

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | channel | string | 是 | wechat/alipay/stripe |
| 输入 | environment | string | 是 | sandbox/production |
| 输入 | credentials | object | 是 | 渠道凭证 |
| 输入 | webhook_url | string | 是 | 回调地址 |
| 输出 | config_id | int64 | 是 | 配置 ID |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/payment-config | 配置列表 |
| GET | /api/payment-config/:channel | 单渠道配置 |
| PUT | /api/payment-config/:channel | 更新配置 |
| POST | /api/payment-config/:channel/test | 测试连接 |
| GET | /api/payment-config/supported | 支持的渠道 |

### 3.3 安全与合规

- 密钥加密
- 仅超级管理员可配置
- 审计日志
- 生产配置二次确认

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 测试连接 | < 3s |
| 配置查询 | < 100ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/payment_config | |
| Service | internal/service/payment_config | |
| Repository | internal/repository/payment_config | |
| Model | internal/model/payment_config | |
| Infra | internal/infra/payment | 各支付 SDK |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 订单 | 使用支付配置 |
| OBS | 证书存储 |

### 4.3 数据流向

```text
[配置变更] → [加密入库] → [支付网关加载] → [调用支付 API]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统 → 支付配置"
2. 选择渠道(微信/支付宝/Stripe)
3. 填写凭证(AppID/MchID/Key 等)
4. 上传证书(可选)
5. 填写 Webhook 回调地址
6. 点击"测试连接"
7. 切换 sandbox → production

### 5.2 系统处理流程

1. 接收配置
2. 加密敏感字段
3. 入库
4. 测试连接:小额支付(沙箱)
5. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 凭证错误 | 500220 | 提示 |
| 证书无效 | 500221 | 提示 |
| 签名错误 | 500222 | 提示 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `payment_configs` | 支付配置 |

```sql
CREATE TABLE payment_configs (
  id BIGINT PRIMARY KEY,
  
  channel VARCHAR(32) NOT NULL,  -- wechat/alipay/stripe
  environment VARCHAR(16) NOT NULL DEFAULT 'sandbox',  -- sandbox/production
  app_id VARCHAR(128),
  mch_id VARCHAR(128),
  credentials_encrypted TEXT,  -- 加密(JSON)
  public_key_encrypted TEXT,
  private_key_encrypted TEXT,
  certificate_url VARCHAR(512),  -- 证书 OBS 路径
  webhook_url VARCHAR(255),
  notify_url VARCHAR(255),
  return_url VARCHAR(255),
  is_enabled BOOLEAN DEFAULT true,
  is_default BOOLEAN DEFAULT false,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  UNIQUE KEY uk_merchant_channel_env ( channel, environment, deleted_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 微信沙箱配置 | 完整 | 成功 | 待执行 |
| TC-002 | 微信生产配置 | 完整 | 二次确认 | 待执行 |
| TC-003 | 支付宝沙箱 | 完整 | 成功 | 待执行 |
| TC-004 | Stripe 配置 | API Key | 成功 | 待执行 |
| TC-005 | 证书上传 | 证书 | 上传成功 | 待执行 |
| TC-006 | 加密存储 | DB | 加密 | 待执行 |
| TC-007 | 测试连接 | 触发 | 成功 | 待执行 |
| TC-008 | 凭证错误 | 错 Key | 失败提示 | 待执行 |
| TC-009 | 切换环境 | 触发 | 加载新 | 待执行 |
| TC-010 | 默认渠道 | 触发 | 单一默认 | 待执行 |
| TC-011 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |
| TC-012 | 禁用渠道 | 触发 | 订单不显示 | 待执行 |
| TC-013 | 回调地址校验 | 错误 URL | 拒绝 | 待执行 |
| TC-014 | Webhook 测试 | 模拟 | 接收 | 待执行 |
| TC-015 | 审计日志 | 修改 | 记录 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 加密密钥 | PAYMENT_AES_KEY | - | |
| 默认环境 | PAYMENT_DEFAULT_ENV | sandbox | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 关键配置变更 | 立即 | 钉钉+短信 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.14 节
- API_CREDENTIALS_CONFIG.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
