# HivemTK 用户端 - 首次启动初始化流程

> 用户端独立部署的初始化流程
> 适用版本：2026-07-21

---

## 一、流程概览

```
docker compose up -d
        │
        ▼
user-server 启动 ──▶ 检查 install.lock
        │                      │
        │                  不存在 / 不合法
        │                      │
        │                      ▼
        │              监听 /api/system/init-license
        │              等待管理员提交 LicenseKey
        │                      │
        │                      ▼
        │              校验 LicenseKey（与 PLATFORM_LICENSE_SECRET 配合 HMAC）
        │                      │
        │                      ▼
        │              写入 install.lock（install_id / company / expires_at / ...）
        │                      │
        │                      ▼
        ▼                      │
user-server 重启校验 install.lock   ◀────────┘
        │
        ▼
   install.lock 合法？
        │
    ┌───┴────┐
   是        否
    │        │
    │        └─▶ 进入 7 天免费试用
    │
    ▼
 监听 /api/system/init-admin
 等待超管账号创建
        │
        ▼
 创建超管（must_change_password=true）
        │
        ▼
 系统就绪
```

---

## 二、关键文件

| 文件 | 作用 |
|------|------|
| `install.lock` | 部署凭证（install_id / LicenseKey 摘要 / 过期时间 / 公司信息）|
| `migrations/015_init_flow_enhancement.sql` | `system_users.must_change_password` 字段 |
| `migrations/016_merchants_key_length.sql` | `merchants.merchant_key` 长度 64 |

---

## 三、详细步骤

### 3.1 启动 user-server

```bash
docker compose up -d user-server
```

容器启动后，user-server 会：

1. 检查 `/app/data/install.lock` 是否存在
2. **若不存在**：
   - 进入**初始化模式**
   - 监听 `/api/system/init-license` 接收 LicenseKey
3. **若存在**：
   - 校验 install.lock 签名（HMAC-SHA256 with `PLATFORM_LICENSE_SECRET`）
   - 校验通过：进入正常工作模式
   - 校验失败：进入**初始化模式**（允许重新绑定 LicenseKey）

### 3.2 浏览器访问初始化页面

访问 `http://<your-server-ip>:8204/setup`：

```
┌────────────────────────────────────────────┐
│  欢迎使用 HivemTK                            │
│  请输入您的 LicenseKey 激活系统                  │
│                                            │
│  LicenseKey: [___________________________]  │
│                                            │
│  [   激活系统   ]                           │
│                                            │
│  试用版说明：未绑定 LicenseKey 时可使用 7 天    │
└────────────────────────────────────────────┘
```

### 3.3 提交 LicenseKey

LicenseKey 是 32 位十六进制字符串，格式 `XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX`（含连字符时为 35 字符）。

校验流程：

1. user-server 接收 LicenseKey
2. 调用平台端 `POST {PLATFORM_API_URL}/api/platform/license/validate`
3. 平台端返回：
   - `valid: true` + License 元数据（company / contact / expires_at）
   - `valid: false` + 原因
4. user-server 校验通过后写入 `install.lock`：
   ```json
   {
     "license_key": "XXXX-XXXX-XXXX-XXXX",
     "install_id": "<UUID v4>",
     "company": "...",
     "contact_email": "...",
     "issued_at": "2026-07-21T00:00:00Z",
     "expires_at": "2027-07-21T00:00:00Z",
     "trial": false,
     "version": "1.0.0",
     "signed_at": "2026-07-21T...",
     "signature": "<HMAC-SHA256>"
   }
   ```

### 3.4 创建超管账号

LicenseKey 绑定成功后，进入「创建超管账号」页面：

```
┌────────────────────────────────────────────┐
│  创建超级管理员                                │
│                                            │
│  用户名: [admin_____________]              │
│  密码:   [_________________]                │
│  确认密码: [_________________]              │
│  姓名:   [_________________]                │
│  邮箱:   [_________________]                │
│                                            │
│  [   创建账号   ]                           │
└────────────────────────────────────────────┘
```

- 密码强度：≥ 12 字符 + 大小写 + 数字 + 特殊字符
- 创建后 `system_users.must_change_password = true`
- 提示：「首次登录后会强制修改密码」

### 3.5 登录

```
http://<your-server-ip>:8204/login
```

使用刚创建的超管账号登录。

**首次登录强制改密**：
- 系统检测到 `must_change_password=true`
- 跳转到「修改密码」页面
- 用户设置新密码后 `must_change_password=false`
- 进入系统主页

### 3.6 系统就绪

完成上述步骤后：

- 系统正式可用
- 所有功能（AI / RAG / 客服 / 营销）按 License 范围开放
- 7 天免费试用结束前会显示告警

---

## 四、7 天免费试用

未绑定商户标识时，user-server 仍可正常运行（开源版无授权限制）：

- 全部功能可用，无使用期限
- 数据持久化（重装后配置仍保留）
- install.lock 模板见 `offline-deploy/install.lock.example`

首次初始化绑定商户标识（merchant_key）：

- 访问 `/setup` 提交商户标识
- 系统校验后写入本地配置，完成初始化

---

## 五、平台端不可达

若平台端暂时不可达（如未配置 `PLATFORM_API_URL` 或网络故障）：

- 试用模式下不受影响
- 正式 LicenseKey 校验失败时，进入**待重试**状态
- 平台端恢复后可重新提交

---

## 六、安全

- `install.lock` 由 `PLATFORM_LICENSE_SECRET` HMAC 签名
- LicenseKey 不可篡改（修改后签名不匹配）
- 迁移机器后 `install.lock` 仍可校验
- 卸载软件会保留 install.lock（在 `/app/data/` 命名卷中），重装不丢

---

## 七、相关代码

- `internal/service/system_init.go` - 初始化流程主逻辑
- `internal/controller/system_init.go` - HTTP API
- `internal/middleware/init_guard.go` - 初始化保护（未绑定 LicenseKey 时拒绝业务请求）
- `internal/model/system_user.go` - system_users model（含 must_change_password）
- `migrations/015_init_flow_enhancement.sql` - 字段迁移
