# 平台账号管理 (Platform Account)

> **所属模块**: unified-message
> **功能 slug**: `platform-account`
> **文档定位**: 多平台账号绑定与登录,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 平台账号管理 |
| 功能名称(英文) | Platform Account |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | unified-message |
| 优先级 | P0 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端账号管理页
- [x] 多平台登录(扫码/手机号/API)
- [x] 健康状态检查
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 反检测与风控对抗

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户需要运营多个平台账号(微信/企微/抖音/小红书/WhatsApp),系统需统一管理这些账号的登录状态、凭证、权限,支持消息收发、自动回复。

### 2.2 解决思路

抽象统一账号模型 `{platform, account_id, nickname, avatar, credentials, status}`,各平台 Adapter 实现登录、心跳、消息收发。

### 2.3 关键算法或模型

- **登录方式**:
  - 微信/企微: 扫码登录(浏览器自动化)
  - 抖音/快手: 扫码 + Cookie 注入
  - 小红书: 扫码
  - WhatsApp: 二维码扫码
  - Telegram: 手机号 + 验证码
- **心跳**: 每 60 秒检查在线状态
- **凭证加密**: Cookie/token AES 加密
- **健康度**: 登录状态 + 最近活跃 + 风控状态

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | platform | string | 是 | 平台标识 |
| 输入 | name | string | 是 | 账号名称 |
| 输入 | login_method | string | 是 | qrcode/phone/api |
| 输出 | account_id | int64 | 是 | 账号 ID |
| 输出 | qrcode_url | string | 否 | 登录二维码 |
| 输出 | status | string | 是 | 当前状态 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/platform-account | 账号列表 |
| POST | /api/platform-account | 创建/添加 |
| GET | /api/platform-account/:id | 详情 |
| PUT | /api/platform-account/:id | 更新 |
| DELETE | /api/platform-account/:id | 删除 |
| POST | /api/platform-account/:id/login | 发起登录 |
| GET | /api/platform-account/:id/qrcode | 获取二维码 |
| POST | /api/platform-account/:id/logout | 登出 |
| GET | /api/platform-account/:id/status | 状态检查 |
| POST | /api/platform-account/:id/heartbeat | 主动心跳 |

### 3.3 安全与合规

- 凭证 AES-256 加密
- 登录过程不存储明文密码
- 风控检测(异常 IP、频繁登录)
- 审计日志

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 二维码生成 | < 1s |
| 状态检查 | < 200ms |
| 心跳检查 | < 500ms |
| 账号登录成功率 | ≥ 95% |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/platform_account | |
| Service | internal/service/platform_account | 账号 CRUD + 登录编排 |
| Engine | internal/service/platform_account/adapters | 各平台 Adapter |
| Engine | internal/service/platform_account/heartbeat | 心跳服务 |
| Repository | internal/repository/platform_account | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 统一消息 | 发送凭证 |
| 自动回复 | 监听 |
| WebSocket | 登录状态推送 |
| 浏览器自动化 | 扫码登录(chromedp) |

### 4.3 数据流向

```text
[用户添加] → [Adapter 启动登录] → [返回二维码]
                                          ↓
[用户扫码] → [Adapter 接收回调] → [保存凭证] → [状态=在线]
                                                            ↓
                                            [定时心跳] → [状态刷新]
```

---

## 五、流程说明

### 5.1 用户操作流程(扫码)

1. 进入"统一消息 → 平台账号"
2. 点击"添加账号"
3. 选择平台 + 填写名称
4. 系统弹出二维码
5. 用对应 APP 扫码确认
6. 登录成功,状态"在线"

### 5.2 用户操作流程(API)

1. 选择平台 + 登录方式 = API
2. 填写 API Key / Token
3. 验证连接
4. 保存

### 5.3 系统处理流程(心跳)

1. 每 60 秒触发心跳
2. 调用各平台 Adapter 的 IsOnline
3. 更新 status
4. 异常时标记并告警

### 5.4 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 登录超时 | 500200 | 重新发起 |
| 凭证失效 | 500201 | 重新登录 |
| 风控拦截 | 500202 | 暂停账号 |

### 5.5 状态机

```text
[离线] → [登录中] → [在线] → [离线]
                    ↓
                  [异常/风控] → [暂停]
```

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `platform_accounts` | 平台账号 |
| `platform_account_logs` | 账号日志 |

```sql
CREATE TABLE platform_accounts (
  id BIGINT PRIMARY KEY,
  
  platform VARCHAR(32) NOT NULL,  -- wechat/wecom/douyin/xiaohongshu/whatsapp/telegram
  name VARCHAR(128) NOT NULL,
  account_unique_id VARCHAR(128),  -- 平台唯一 ID
  nickname VARCHAR(128),
  avatar_url VARCHAR(512),
  phone VARCHAR(32),  -- 脱敏
  credentials_encrypted TEXT,  -- 凭证加密
  status VARCHAR(16) DEFAULT 'offline',  -- offline/logging_in/online/error/suspended
  health_score INT DEFAULT 100,  -- 0-100 健康度
  last_active_at TIMESTAMP,
  last_heartbeat_at TIMESTAMP,
  error_message TEXT,
  is_risk BOOLEAN DEFAULT false,  -- 风控标记
  daily_message_count INT DEFAULT 0,
  daily_message_limit INT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_data, platform, deleted_at),
  INDEX idx_status (status)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 微信扫码登录 | 触发 | 登录成功 | 待执行 |
| TC-002 | 企微扫码登录 | 触发 | 登录成功 | 待执行 |
| TC-003 | 抖音扫码登录 | 触发 | 登录成功 | 待执行 |
| TC-004 | WhatsApp 登录 | 触发 | 登录成功 | 待执行 |
| TC-005 | Telegram 手机号 | 验证码 | 登录成功 | 待执行 |
| TC-006 | 凭证加密 | DB | 加密 | 待执行 |
| TC-007 | 状态检查 | 触发 | 正确状态 | 待执行 |
| TC-008 | 心跳检测 | 60s 间隔 | 状态刷新 | 待执行 |
| TC-009 | 凭证失效 | 模拟 | 标记 error | 待执行 |
| TC-010 | 登出 | 触发 | 状态=offline | 待执行 |
| TC-011 | 重新登录 | error 状态 | 重新发起 | 待执行 |
| TC-012 | 风控检测 | 异常 | 暂停 | 待执行 |
| TC-013 | 每日限额 | 超限 | 拒绝发送 | 待执行 |
| TC-014 | 健康度评分 | 多次失败 | 分数降低 | 待执行 |
| TC-015 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |
| TC-016 | 二维码过期 | 5min 后 | 提示刷新 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 心跳间隔 | PLATFORM_ACCOUNT_HEARTBEAT | 60s | |
| 加密密钥 | PLATFORM_ACCOUNT_AES_KEY | - | |
| 浏览器实例 | CHROMEDP_INSTANCES | 5 | |
| 二维码过期 | QRCODE_EXPIRE | 5min | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 账号掉线 | 立即 | 钉钉 |
| 风控触发 | 立即 | 短信+钉钉 |
| 健康度低 | < 60 | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.13 节
- BROWSER_ASSISTANT.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
