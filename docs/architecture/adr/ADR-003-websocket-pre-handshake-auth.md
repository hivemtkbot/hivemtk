# ADR-003: WebSocket 握手前强制鉴权

- **状态**：已采纳（2026-07-24 起本地/私域部署撤销 agent WS 握手前鉴权 —— 见末尾"撤销说明"）
- **日期**：2026-07
- **范围**：所有 WebSocket 端点（user-server + platform-server）

## 背景

`internal/websocket/handler.go` 在 `upgrader.Upgrade` 之前仅校验 `agent_id` query 参数，
不校验 token；任意第三方拿到 WebSocket URL 即可建立连接并占用资源，构成未授权访问漏洞。

## 决策

1. WebSocket 握手前必须解析并校验 token（query `?token=...` 或 `Authorization: Bearer ...` 头）
2. token 中 `user_id` 必须与 query `agent_id` 严格对齐，防止越权占用他人连接
3. 校验失败立即返回 401/403 JSON 响应，不进入 `upgrader.Upgrade`

## 落地

- `internal/websocket/handler.go:HandleWebSocket` 在 Upgrade 之前调用 `utils.NewJWTUtils().ParseToken`
- 配套 `docs/TROUBLESHOOTING.md` §2.2 给出前端集成示例

## 备选

- subprotocol 携带 token：浏览器 WebSocket API 不友好，舍弃
- 长连接建立后第一帧携带 token：增加一次空往返，且与"握手前鉴权"语义不符，舍弃

## 影响

- 浏览器集成需在 `new WebSocket(url)` 时附加 `?token=${token}`（WebSocket API 不支持自定义 header）
- 服务端不再接受匿名连接，CC 攻击 / 资源耗尽风险显著下降

## 撤销说明（2026-07-24，本地/私域部署）

本地/私域部署与"关闭 CORS"保持一致：**agent WebSocket 入口不再做 token 鉴权**。
`internal/websocket/handler.go:HandleWebSocket` 仅依据 query `agent_id` 路由身份，
移除 `utils.NewJWTUtils().ParseToken` 校验与 `user_id == agent_id` 对齐校验。

依据：
- 私域单租户、内网访问，无第三方站点发起未授权跨域连接的风险；
- 访客侧 WebSocket（`HandleVisitorWebSocket`）本就按设计不做鉴权，两侧保持一致。

**安全边界**：本地部署下任何能访问服务端口者均可占用指定 agent 的连接并收发消息。
仅限可信内网 / 单机私域。公网 / 多租户场景须恢复握手前 token 鉴权。
