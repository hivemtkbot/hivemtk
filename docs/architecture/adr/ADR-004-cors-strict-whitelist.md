# ADR-004: CORS 白名单精确匹配 + 凭证条件启用

- **范围**：所有 Gin 服务（user-server + platform-server）

## 背景

`cors.go` 历史实现反射任意 Origin + 始终携带 `Access-Control-Allow-Credentials: true`，
配合 CSRF 攻击可让用户浏览器在不知情情况下代为发起跨域带 Cookie 请求。
即使内部白名单为空，反射 Origin 仍构成严重 CSRF 风险。

## 决策

1. CORS 中间件强制白名单精确匹配；不在白名单的 Origin 立即 403，**不写 CORS 响应头**
2. `Access-Control-Allow-Credentials: true` 仅在 Origin 命中白名单时设置
3. 禁止 `Access-Control-Allow-Origin: *` 与 Allow-Credentials 共存（违反 CORS 规范，浏览器会拒绝）
4. 白名单通过 `CORS_ALLOW_ORIGINS` 环境变量（逗号分隔）注入；`cors.json` 兜底默认

## 落地

- `internal/middleware/cors.go` 重写为白名单模式
- `pkg/utils/config/cors.go` 暴露 `GetCORSOrigins()` API
- `docs/TROUBLESHOOTING.md` §2.1 给出新增白名单 Origin 的步骤

## 影响

- 任何接入方必须显式注册到白名单；新部署后忘记配置 CORS 会"看起来像故障"，但比 CSRF 安全
- `Vary: Origin` 头确保 CDN 不会混淆不同 Origin 的缓存

## 撤销说明（2026-07-24，本地/私域部署）

本仓库面向私有化自部署场景：user-web 由 user-server 同源静态托管、浏览器直连本机 API，
不存在第三方站点跨域读取用户凭证的攻击面。因此**本地/私域部署不再启用 CORS 中间件**：

- 删除 `internal/middleware/cors.go`、`pkg/utils/config/cors.go`（及测试）
- 移除 `cmd/api/main.go` 的 `corsAllowAll()` 与 `router.go` 的 `middleware.CORS()` 注册
- platform-server 同步移除 `middleware.CORS()` 注册与 `logger.go` 的 `CORS()` 函数

安全审计项 `CORS 配置` 文案改为"本地/私域部署不启用 CORS，该项不适用"。

**适用边界**：仅限可信内网 / 单机私域部署。若后续要做公网多租户 SaaS，须恢复白名单 CORS
（且绝不可让 `Allow-Origin: *` 与 `Allow-Credentials: true` 共存）。
