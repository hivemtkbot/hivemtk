# user-web/bridge 代码开发手册（HiveBridge 蜂桥）

> **规则级别**: ⭐⭐ 项目级开发文档
>
> 本手册面向 `user-web/bridge`（HiveBridge 蜂桥 Chrome 扩展）的二次开发者，覆盖环境准备、命令、目录导航、协议契约、新功能流程、测试、构建发布与调试技巧。
>
> 关联文档：
> - 扩展设计主文档：[./bridge.md](./bridge.md)
> - 扩展默认值清单：[./docs/DEFAULTS.md](./docs/DEFAULTS.md)
> - 部署安装文档（frp）：[../../docs/operations/FRP私域部署指南.md](../../docs/operations/FRP私域部署指南.md)
> - 上游服务端 WS 实现：[../../../user-server/internal/bridge/handler.go](../../../user-server/internal/bridge/handler.go)

---

## 一、环境准备

| 依赖 | 版本 | 说明 |
| --- | --- | --- |
| Node.js | ≥ 20 | 测试与构建统一使用 Node 20 LTS |
| npm | ≥ 10 | 仓库内置 `package-lock.json`，推荐 `npm install` |
| esbuild | ^0.21.5 | `npm run build` 通过 esbuild 多入口打包（MV3 content/background 场景最稳） |
| Vitest | ^1.6.0 | 单元测试，jsdom 环境 |
| Chrome | ≥ 110 | 加载已解压扩展进行联调（MV3） |
| user-server | 本地或远端 | 默认 `http://localhost:8204`（单一源：`src/core/constants.js`） |

初始化：

```bash
cd /Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/bridge
npm install
```

---

## 二、启动命令

| 命令 | 作用 | 备注 |
| --- | --- | --- |
| `npm run build` | esbuild 多入口打包到 `dist/` | 每次修改源码必跑，扩展加载的是 dist 产物 |
| `npm run dev` | `node scripts/build.mjs --watch` 持续构建 | 监听源码变化，刷新扩展生效 |
| `npm run test` | Vitest 单元测试（jsdom） | 覆盖 `constants.js` / `protocol` / `adapter` / `rate-limiter` / `sanitize` / `fallback` / `popup` |
| `npm run test:watch` | 监听模式 | 开发期间持续运行 |
| `npm run release` | `node scripts/release.mjs` 打 release 包 | 产物 `release/hivebridge-1.0.0.zip` |

### 2.1 本地开发联调（核心三步）

```bash
# 1. 启动 user-server（必先启动）
cd ../../user-server && go run ./cmd/api
# 验证：curl http://localhost:8204/health

# 2. 构建扩展产物
cd user-web/bridge
npm run dev    # 持续构建（推荐）

# 3. Chrome 加载扩展
# Chrome → chrome://extensions → 开启「开发者模式」→ 「加载已解压扩展」→ 选择 user-web/bridge/dist/
# 修改源码后只需点击 chrome://extensions 的「重新加载」按钮
```

### 2.2 启动后验证

```bash
# 1. 扩展 popup 打开后「测试连接」按钮
#    默认 server = http://localhost:8204（与 user-server 端口对齐）
#    应弹出「连接成功」

# 2. 打开抖音/小红书/TikTok 私信页
#    F12 → Console 看到 [bridge] ... 日志
#    F12 → Application → Service Workers → 查看 background.js 日志
```

---

## 三、端口对照表

> **核心约束**：`src/core/constants.js` 的 `DEFAULT_USER_SERVER.port` = **8204**（user-server Gin HTTP 端口）
> 这是扩展**唯一**的默认服务端地址。任何前端代码、文档、配置均不得修改此字面量，除非同步修改 user-server `internal/config/ports.go` 的 `DefaultListenPort` 与本文件。

| 端口 | 服务 / 应用 | 启动入口 | 单一源常量 | 文档源 |
| --- | --- | --- | --- | --- |
| **8204** | **user-server**（扩展连接入口） | `go run ./cmd/api` | `constants.DEFAULT_USER_SERVER.port` | user-server `internal/config/ports.go` `DefaultListenPort` |
| 8207 | LLM（本地推理） | 扩展不直连 | user-server `DefaultLLMPort` | user-server docs §2.4 |
| 8208 | Embedding（本地推理） | 扩展不直连 | user-server `DefaultEmbeddingPort` | user-server docs §2.4 |
| 8209 | Rerank（本地推理） | 扩展不直连 | user-server `DefaultRerankPort` | user-server docs §2.4 |

### 3.1 popup 端口默认值

- **默认**：`http://localhost:8204`（`DEFAULT_USER_SERVER.baseUrl`）
- 覆盖方式：在 popup 表单中手动修改，存储到 `chrome.storage.local`
- 跨平台：用户远程部署时改为 `https://user-server.your-domain.com` 等公网地址

### 3.2 WS 端点

- 默认：`/api/ws/bridge`（`DEFAULT_USER_SERVER.wsPath`）
- 服务端注册：`user-server/internal/router/service_routes.go:90` `auth.GET("/ws/bridge", ...)`
- 鉴权：通过 URL query `?token=<JWT>`（popup 自动从 localStorage 读取 user-server 端 JWT）

---

## 四、目录导航

```
user-web/bridge/
├── src/
│   ├── background/                   后台 service_worker
│   │   ├── index.js                  入口：WS 客户端 + 路由注册表
│   │   └── registry.js               路由注册表（devicelistener / onUpdated）
│   ├── channels/                     各平台适配层
│   │   ├── douyin.js                 抖音 IM DOM 解析
│   │   ├── xhs.js                    小红书 DOM 解析
│   │   └── tiktok.js                 TikTok DOM 解析
│   ├── content/                      content script
│   │   ├── common.js                 公共：DOM 监听 / 消息路由
│   │   ├── douyin.js                 抖音 content 入口
│   │   ├── tiktok.js                 TikTok content 入口
│   │   └── xhs.js                    小红书 content 入口
│   ├── core/                         核心模块（与 platform 无关）
│   │   ├── bridge-client.js          WS 客户端（重连 / seq / ack）
│   │   ├── channel-adapter.js        渠道 → 统一消息适配器
│   │   ├── constants.js              ⭐ 单一源：所有默认值（端口/URL/限速/协议）
│   │   ├── dom.js                    DOM 工具
│   │   ├── fallback.js               兜底回复（关键词匹配）
│   │   ├── logger.js                 日志脱敏
│   │   ├── rate-limiter.js           三层风控（拟人节奏 + 令牌桶 + 会话冷却 + 去重）
│   │   ├── sanitize.js               XSS 防护 / 内容净化
│   │   └── types.js                  协议类型定义
│   ├── popup/                        popup UI
│   │   ├── index.html
│   │   └── index.js
├── test/                             Vitest 单元测试
│   ├── constants.test.js             ⭐ 验证单一源字面一致
│   ├── protocol.test.js              协议帧测试
│   ├── adapter.test.js               渠道适配器测试
│   ├── rate-limiter.test.js          限速桶测试
│   ├── sanitize.test.js              XSS 防护测试
│   ├── fallback.test.js              兜底回复测试
│   ├── bridge-client.test.js         WS 客户端测试
│   ├── background.test.js            后台测试
│   └── popup.test.js                 popup UI 测试
├── docs/
│   └── DEFAULTS.md                   ⭐ 前端默认值单一文档源
├── dist/                             esbuild 构建产物（不入仓）
├── scripts/
│   ├── build.mjs                     esbuild 多入口打包
│   └── release.mjs                   release 打包
├── manifest.json                     MV3 manifest
├── package.json
├── vite.config.js                    Vitest 配置
└── bridge.md                         设计主文档
```

### 4.1 关键文件作用

| 文件 | 作用 |
| --- | --- |
| `src/core/constants.js` | **单一源**：端口 / URL / 限速 / WS 协议 / 安全护栏。**所有模块从这里 import，禁止就地写数字** |
| `src/core/bridge-client.js` | WS 客户端：指数退避重连 + 25s 心跳超时 + seq/ack 帧处理 |
| `src/core/rate-limiter.js` | 三层风控：拟人节奏（1.5s 最小间隔 + 800-2600ms 抖动）+ 令牌桶（12/min）+ 会话冷却（3s）+ 60s 去重窗口 |
| `src/core/sanitize.js` | XSS 防护：剥离控制字符 + 截断 4KB（与服务端 `maxReplyContentBytes` 对齐） |
| `src/background/index.js` | 后台 service_worker：维护 WS 连接、路由 content 消息、调度回复 |
| `src/content/*.js` | content script：DOM 监听、抽取消息、调用 channel-adapter 标准化 |
| `src/popup/index.js` | popup UI：账号配置、测试连接、状态显示 |

---

## 五、协议契约（与服务端一一对应）

> **强约束**：扩展端 `FRAME` / `SENDER` / `DIRECTION` 枚举必须与 `user-server/internal/bridge/frames.go` 字面一致。
> 任何新增/修改必须同步两端 + 文档（`bridge.md` §3 + `docs/DEFAULTS.md` §协议常量）。

```javascript
// src/core/constants.js
export const PROTOCOL = Object.freeze({
  CHANNELS: { DOUYIN: 'douyin_web', XHS: 'xhs_web', TIKTOK: 'tiktok_web' },
  FRAME: {
    REGISTER: 'register',
    INBOUND: 'inbound_message',
    HISTORY: 'history',
    OUTBOUND: 'outbound_reply',
    PONG: 'pong',
    PING: 'ping',
    ACK: 'ack',
    ERROR: 'error',
  },
  SENDER: { CUSTOMER: 'customer', AGENT: 'agent', SELF: 'self' },
  DIRECTION: { INBOUND: 'inbound', OUTBOUND: 'outbound' },
});
```

---

## 六、新功能标准流程

### 6.1 新增前端常量（端口/URL/限速）

1. 修改 `src/core/constants.js`，确保新值有**文档源**（DEVELOPMENT.md 或 DEFAULTS.md 链接）
2. 同步 `docs/DEFAULTS.md` 表格
3. 在 `test/constants.test.js` 追加断言（验证字面与文档源一致）
4. 如跨包（如 user-server 端口）：同步 `user-server/internal/config/ports.go`
5. 跑 `npm run test` 验证

### 6.2 新增渠道（如快手私信）

1. 在 `src/channels/` 新增 `kuaishou.js`，实现 DOM 解析与统一消息转换
2. 在 `src/core/channel-adapter.js` 注册新渠道
3. 在 `src/content/` 新增 `kuaishou.js` content script
4. 修改 `manifest.json`：
   - `content_scripts` 添加 `matches: ["https://*.kuaishou.com/*"]` + `js: ["content-kuaishou.js"]`
   - `host_permissions` 已有 `http://*/*` / `https://*/*`，无需新增
5. 在 `src/popup/index.html` 添加「打开快手」入口
6. 跑 `npm run test && npm run build` 验证

### 6.3 调整限速/风控参数

1. 修改 `src/core/constants.js` 的 `RATE_LIMIT_DEFAULTS` 数值
2. 同步 `docs/DEFAULTS.md` §限速风控表
3. 在 `test/rate-limiter.test.js` 验证行为变更
4. 跑 `npm run test` 验证

---

## 七、测试运行

### 7.1 单元测试

```bash
npm run test        # 一次性运行
npm run test:watch  # 监听模式
```

**测试覆盖范围**（`test/` 目录）：

| 测试文件 | 覆盖模块 |
| --- | --- |
| `constants.test.js` | 端口/URL/限速/协议常量与 DEFAULTS.md 字面一致 |
| `protocol.test.js` | FRAME / SENDER / DIRECTION 帧类型 |
| `adapter.test.js` | 渠道适配器（DOM → UnifiedMessage） |
| `rate-limiter.test.js` | 三层风控（节奏 / 桶 / 冷却 / 去重） |
| `sanitize.test.js` | XSS 防护（控制字符 / 4KB 截断） |
| `fallback.test.js` | 兜底回复（关键词匹配） |
| `bridge-client.test.js` | WS 客户端（重连 / 心跳 / seq/ack） |
| `background.test.js` | 后台路由 |
| `popup.test.js` | popup UI 交互 |

---

## 八、构建与发布

### 8.1 本地构建

```bash
npm run build
# 产物：dist/background.js, dist/content-{douyin,xhs,tiktok}.js, dist/popup.{html,js}, dist/manifest.json
```

### 8.2 发布打包

```bash
npm run release
# 产物：release/hivebridge-1.0.0.zip
# 内容：dist/ 全部 + assets/logo.svg + LICENSE
```

### 8.3 Chrome 加载

1. 打开 `chrome://extensions`
2. 开启「开发者模式」
3. 「加载已解压扩展」→ 选择 `dist/` 目录
4. 修改源码后点击「重新加载」即可

---

## 九、调试技巧

### 9.1 popup 无法连接

- **症状**：popup 点击「测试连接」提示失败
- **排查**：
  ```bash
  # 1. 确认 user-server 在 8204 端口运行
  curl http://localhost:8204/health
  
  # 2. 确认扩展 popup 的 server URL 与 user-server 端口一致
  #    单一源：src/core/constants.js DEFAULT_USER_SERVER.baseUrl
  ```

### 9.2 content script 未注入抖音页

- **症状**：打开 douyin.com 私信页，控制台无 `[bridge]` 日志
- **排查**：
  - `chrome://extensions` → HiveBridge → 检查「已启用」开关
  - `chrome://extensions` → 「检查视图」是否有报错
  - F12 → Console → 输入 `window.__bridgeAdapter` 查看是否注入

### 9.3 WS 频繁断连

- **症状**：后台日志显示「连接断开」+ 持续重连
- **排查**：
  ```bash
  # 1. user-server JWT 是否过期（popup 重新登录）
  # 2. user-server 日志中 /api/ws/bridge 是否正常
  tail -f logs/user-server.log | grep ws/bridge
  ```
- 心跳超时：`WS_CLIENT_DEFAULTS.serverIdleTimeoutMs = 25s`，< 服务端 `pongWait=60s`

### 9.4 限速拦截

- **症状**：AI 回复没发出去
- **排查**：
  - F12 → Console → 搜索 `[rate-limiter]`，查看被哪个规则拦截
  - 检查 `RATE_LIMIT_DEFAULTS` 参数是否需要调整

### 9.5 强制重置扩展状态

```javascript
// 在 popup F12 Console 执行
chrome.storage.local.clear()
// 然后 chrome://extensions → 「重新加载」
```

---

## 十、跨包对齐清单

> **任何修改以下字段都必须同步两端**（扩展端 ↔ user-server 端）：

| 字段 | 扩展端 | user-server 端 |
| --- | --- | --- |
| WS 路径 | `constants.DEFAULT_USER_SERVER.wsPath` | `internal/router/service_routes.go:90` |
| user-server 端口 | `constants.DEFAULT_USER_SERVER.port` | `internal/config/ports.go` `DefaultListenPort` |
| 心跳超时 | `WS_CLIENT_DEFAULTS.serverIdleTimeoutMs=25s` | `internal/bridge/handler.go` `pongWait=60s` |
| 最大回复字节 | `SECURITY.maxReplyContentBytes=4KB` | `internal/bridge/handler.go` `maxReplyContentBytes=4KB` |
| 协议帧名 | `PROTOCOL.FRAME.*` | `internal/bridge/frames.go` `Frame*` |
| 健康检查路径 | `DEFAULT_USER_SERVER.healthPaths` | `internal/router/router.go` 实际注册 |

---

## 十一、相关文档

- [./bridge.md](./bridge.md) — 设计主文档（背景 / 架构 / 数据流 / 限速 / 协议）
- [./docs/DEFAULTS.md](./docs/DEFAULTS.md) — 前端默认值清单（与 constants.js 字面对齐）
- [./RELEASE.md](./RELEASE.md) — 发布说明
- [../../../user-server/docs/dev/DEVELOPMENT.md](../../../user-server/docs/dev/DEVELOPMENT.md) — user-server 启动文档
- [../../docs/operations/FRP私域部署指南.md](../../docs/operations/FRP私域部署指南.md) — 远程部署 frp 穿透

---

最近更新日期: 2026-07-28
