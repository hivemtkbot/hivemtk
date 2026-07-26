# 闲鱼自动回复 (Xianyu Auto Reply)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `auto-reply-xianyu`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 闲鱼自动回复 |
| 功能名称（英文） | Xianyu Auto Reply |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | auto-reply |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构
- [x] 后端 Service 与 Controller
- [x] 闲鱼专用规则模板（议价/在吗/包邮等）
- [x] chromedp 闲鱼页面适配
- [x] 卡片快捷回复
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

闲鱼作为二手交易平台，用户咨询集中在议价、商品详情、发货时间等问题。通用自动回复无法覆盖闲鱼的页面结构和消息特征。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_id | int64 | 是 | 闲鱼账号ID |
| 输入 | message | string | 是 | 收到的消息 |
| 输入 | product_id | string | 否 | 当前咨询的商品 |
| 输出 | reply | string | 是 | 回复内容 |
| 输出 | reply_type | string | 是 | text/card/bargain |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/xianyu-auto-reply/accounts | 账号列表 |
| POST | /api/xianyu-auto-reply/accounts | 添加账号 |
| POST | /api/xianyu-auto-reply/login | 登录 |
| GET | /api/xianyu-auto-reply/rules | 规则列表 |
| POST | /api/xianyu-auto-reply/rules | 创建规则 |
| GET | /api/xianyu-auto-reply/logs | 日志 |
| PUT | /api/xianyu-auto-reply/status | 启停 |

### 3.3 安全与合规

- Cookie 加密
- 议价区间限制（最高 20%）
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 消息响应延迟 | < 5s | ~2.5s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/xianyu_auto_reply.go` | 接口 |
| Service | `internal/service/xianyu_auto_reply_service.go` | 业务 |
| Repository | `internal/repository/xianyu_auto_reply_repo.go` | 数据 |
| Model | `internal/model/xianyu_auto_reply.go` | 模型 |
| Infra | `internal/browser/chromedp.go` + Redis | 浏览器+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| card-xianyu | 闲鱼卡片 |
| auto-reply-universal | 复用引擎 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点 |

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加闲鱼账号
2. 选择预设模板（议价/在吗等）
3. 调整还价策略
4. 启动服务
5. 查看日志

### 5.2 系统处理流程

1. chromedp 检测新消息
2. 关键词匹配
3. 议价场景 → 价格区间判断
4. 拼装回复（支持卡片）
5. chromedp 发送
6. 写日志

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 商品已下架 | 404001 | 提示商品不可用 |
| 议价超限 | 400101 | 拒绝议价 |
| 账号掉线 | 401001 | 提示重新登录 |

---

## 六、数据库设计

### 6.1 核心表 xianyu_auto_reply_accounts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_name | varchar(64) | 非空 | 账号 |
| cookie_enc | text | | 加密 Cookie |
| status | tinyint | 非空 | 状态 |

### 6.2 核心表 xianyu_auto_reply_rules

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| template_type | varchar(32) | | bargain/in_stock/shipping |
| reply_content | text | 非空 | 回复 |
| bargain_min_pct | int | | 最低还价比例 |
| bargain_max_pct | int | | 最高还价比例 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 议价场景 | "100 包邮吗" | 还价 90 | ✅ |
| TC-002 | 在吗 | "在吗" | 触发"在的"模板 | ✅ |
| TC-003 | 卡片发送 | 商品咨询 | 发送商品卡片 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| BARGAIN_MAX_PCT | BARGAIN_MAX_PCT | 20 |

---

## 九、参考资料

- [auto-reply-universal.md](auto-reply-universal.md)
- [card-xianyu.md](card-xianyu.md)
