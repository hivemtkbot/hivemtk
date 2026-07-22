# 订单管理 (Order Management)

> **所属模块**: order-payment
> **功能 slug**: `order-management`
> **文档定位**: 订单全生命周期管理,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 订单管理 |
| 功能名称(英文) | Order Management |
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
- [x] 前端订单管理页
- [x] 订单 CRUD + 支付回调
- [x] 退款流程
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 分销订单

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

营销转化最终体现在订单上。商户需要一个订单系统管理支付前的下单、支付中的回调、支付后的履约/退款全流程。

### 2.2 解决思路

标准订单模型(订单/订单项/支付/退款四表),通过统一的支付网关(微信/支付宝/Stripe)接入,Webhook 回调更新状态。

### 2.3 关键算法或模型

- **订单号生成**: `{yyyymmdd}{6位随机}`
- **金额计算**: 商品金额 - 优惠 + 税费 + 运费
- **幂等性**: 支付回调使用 `out_trade_no` 幂等
- **状态机**: pending → paid → fulfilled → completed;pending → cancelled;paid → refunding → refunded
- **库存**: 简化版,记录数量(不强扣减)

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | customer_id | int64 | 是 | 客户 ID |
| 输入 | items | array | 是 | 订单项 |
| 输入 | payment_method | string | 是 | wechat/alipay/stripe |
| 输出 | order_id | int64 | 是 | 订单 ID |
| 输出 | pay_url | string | 是 | 支付 URL |
| 输出 | amount | decimal | 是 | 订单金额 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/order | 订单列表 |
| GET | /api/order/recent | 最近订单 |
| GET | /api/order/:id | 订单详情 |
| POST | /api/order | 创建订单 |
| POST | /api/order/:id/pay | 发起支付 |
| POST | /api/order/:id/check-pay | 检查支付 |
| POST | /api/order/:id/cancel | 取消订单 |
| POST | /api/order/:id/refund | 申请退款 |
| DELETE | /api/order/:id | 删除订单 |
| POST | /api/order/webhook/:channel | 支付回调 |

### 3.3 安全与合规

- 支付签名验证
- Webhook 幂等
- 敏感金额不返回明文银行卡
- 退款双重确认
- 审计日志

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 创建订单 | < 200ms |
| 支付回调 | < 500ms |
| 列表查询 | < 300ms |
| 并发下单 | ≥ 200 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/order | |
| Service | internal/service/order | 订单生命周期 |
| Engine | internal/service/order/payment | 支付网关 |
| Repository | internal/repository/order | |
| Model | internal/model/order | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 支付配置 | 支付渠道凭证 |
| 客户 | 下单人 |
| 商品/产品 | 订单项 |

### 4.3 数据流向

```text
[创建订单] → [选择支付方式] → [调用支付网关] → [返回 pay_url]
                                                              ↓
[用户支付] → [支付平台回调] → [校验签名] → [更新状态] → [触发履约]
                                                                          ↓
                                                                  [申请退款] → [网关退款] → [更新状态]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择商品 → 加入购物车
2. 结算 → 创建订单
3. 选择支付方式 → 发起支付
4. 跳转支付页面
5. 支付成功 → 回调更新
6. 等待履约/查看物流

### 5.2 系统处理流程(支付回调)

1. 接收 Webhook
2. 校验签名
3. 幂等检查(`out_trade_no`)
4. 更新订单状态
5. 触发履约(发货等)
6. 返回 200 OK

### 5.3 系统处理流程(退款)

1. 用户申请退款
2. 商户审核(可选)
3. 调用支付网关退款
4. 接收退款回调
5. 更新订单状态
6. 通知用户

### 5.4 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 签名错误 | 400170 | 拒绝 |
| 订单不存在 | 404090 | 404 |
| 重复回调 | - | 幂等返回 |
| 金额不一致 | 500210 | 风控告警 |
| 支付超时 | - | 自动取消(30min) |

### 5.5 状态机

```text
[待支付] → [已支付] → [已发货] → [已完成]
     ↓
   [已取消]   ↓
          [退款中] → [已退款]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `orders` | 订单主表 |
| `order_items` | 订单项 |
| `order_payments` | 支付记录 |
| `order_refunds` | 退款记录 |

```sql
CREATE TABLE orders (
  id BIGINT PRIMARY KEY,
  
  order_no VARCHAR(64) NOT NULL UNIQUE,
  customer_id BIGINT NOT NULL,
  customer_name VARCHAR(128),
  total_amount DECIMAL(15,2) NOT NULL,
  discount_amount DECIMAL(15,2) DEFAULT 0,
  shipping_fee DECIMAL(15,2) DEFAULT 0,
  tax_fee DECIMAL(15,2) DEFAULT 0,
  paid_amount DECIMAL(15,2) DEFAULT 0,
  refunded_amount DECIMAL(15,2) DEFAULT 0,
  currency VARCHAR(8) DEFAULT 'CNY',
  status VARCHAR(16) DEFAULT 'pending',  -- pending/paid/fulfilled/completed/cancelled/refunding/refunded
  payment_method VARCHAR(32),
  payment_status VARCHAR(16) DEFAULT 'unpaid',
  payment_at TIMESTAMP,
  cancel_reason TEXT,
  out_trade_no VARCHAR(64),
  remark TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, status, created_at),
  INDEX idx_customer (customer_id, created_at)
);

CREATE TABLE order_items (
  id BIGINT PRIMARY KEY,
  order_id BIGINT NOT NULL,
  product_id BIGINT,
  product_name VARCHAR(255) NOT NULL,
  product_image VARCHAR(512),
  price DECIMAL(15,2) NOT NULL,
  quantity INT NOT NULL,
  subtotal DECIMAL(15,2) NOT NULL,
  INDEX idx_order (order_id)
);

CREATE TABLE order_payments (
  id BIGINT PRIMARY KEY,
  order_id BIGINT NOT NULL,
  payment_method VARCHAR(32) NOT NULL,
  out_trade_no VARCHAR(64) UNIQUE,
  transaction_id VARCHAR(128),
  amount DECIMAL(15,2) NOT NULL,
  status VARCHAR(16) NOT NULL,  -- pending/success/failed
  paid_at TIMESTAMP,
  raw_response JSONB,
  INDEX idx_order (order_id)
);

CREATE TABLE order_refunds (
  id BIGINT PRIMARY KEY,
  order_id BIGINT NOT NULL,
  refund_no VARCHAR(64) UNIQUE,
  amount DECIMAL(15,2) NOT NULL,
  reason TEXT,
  status VARCHAR(16) DEFAULT 'pending',
  refund_id VARCHAR(128),
  processed_at TIMESTAMP,
  INDEX idx_order (order_id)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建订单 | 完整参数 | 订单号 | 待执行 |
| TC-002 | 金额计算 | 多商品 | 正确合计 | 待执行 |
| TC-003 | 微信支付 | 触发 | pay_url | 待执行 |
| TC-004 | 支付宝支付 | 触发 | pay_url | 待执行 |
| TC-005 | Stripe 支付 | 触发 | pay_url | 待执行 |
| TC-006 | 支付回调 | 模拟 | 状态=paid | 待执行 |
| TC-007 | 重复回调 | 多次 | 幂等 | 待执行 |
| TC-008 | 签名验证 | 错误签名 | 拒绝 | 待执行 |
| TC-009 | 取消订单 | 待支付 | 状态=cancelled | 待执行 |
| TC-010 | 申请退款 | 已支付 | 退款中 | 待执行 |
| TC-011 | 退款完成 | 模拟 | 已退款 | 待执行 |
| TC-012 | 订单删除 | 软删除 | 列表不可见 | 待执行 |
| TC-013 | 订单详情 | ID | 完整 | 待执行 |
| TC-014 | 最近订单 | 7 天 | 列表 | 待执行 |
| TC-015 | 超时取消 | 30min | 自动取消 | 待执行 |
| TC-016 | 金额不一致 | 模拟 | 告警 | 待执行 |
| TC-017 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |
| TC-018 | 列表分页 | 100 条 | 正确 | 待执行 |
| TC-019 | 状态过滤 | pending | 仅待支付 | 待执行 |
| TC-020 | 导出 | CSV | 完整 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 支付超时 | ORDER_PAY_TIMEOUT | 30min | |
| 货币 | ORDER_DEFAULT_CURRENCY | CNY | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 支付失败率 | > 5% | 钉钉 |
| 回调异常 | > 1% | 钉钉 |
| 退款异常 | 立即 | 邮件+短信 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.14 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
