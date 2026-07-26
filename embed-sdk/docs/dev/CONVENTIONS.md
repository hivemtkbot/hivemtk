# embed-sdk 代码规范

> **规则级别**: ⭐⭐ 项目级开发文档

> 本规范约束 embed-sdk 项目的命名、文件、样式、配置、通信、安全、兼容性、体积与禁止清单。所有 PR 必须遵守。
> 关联文档：[../../README.md](../../README.md)、[./ARCHITECTURE.md](./ARCHITECTURE.md)、[./DEVELOPMENT.md](./DEVELOPMENT.md)、[../../../docs/architecture/adr/ADR-011-chat-widget-embed.md](../../../docs/architecture/adr/ADR-011-chat-widget-embed.md)

---

## 一、命名规范

### 1.1 JavaScript 变量与函数

- 一律使用 `camelCase`：`apiBaseURL`、`allowedOrigins`、`getChannelRef`、`setUnread`、`parseConfig`
- 类名使用 `PascalCase`：`MarketingChatWidget`、`FloatingButton`、`IframePanel`
- 常量使用 `UPPER_SNAKE_CASE`：`DEFAULTS`、`ICON_SVG`、`CLOSE_SVG`
- 私有方法以 `_` 前缀：`_fireEvent`、`_bindMessageListener`、`_onWindowMessage`、`_vv`、`_vvApply`、`_innerHTML`、`_cssText`、`_listeners`
- JSDoc `@typedef` 类型名使用 `PascalCase` 并加 `Mcw` 前缀避免与宿主代码冲突：`McwConfig`、`McwEvents`、`McwPosition`、`McwMessage`、`FloatingButtonOptions`、`IframePanelOptions`

### 1.2 CSS 类名

所有 CSS 类名必须以 `mcw-` 前缀（marketing-chat-widget 缩写），避免污染宿主页面：

| 类名 | 出现位置 | 用途 |
|---|---|---|
| `.mcw-floating-btn` | `floating-button.js#mount` | 浮标按钮容器 |
| `.mcw-open` | `floating-button.js#mount`（toggle） | 浮标激活态（图标切换为关闭） |
| `.mcw-fab-icon` | `floating-button.js#mount` | 浮标内部图标 span |
| `.mcw-fab-badge` | `floating-button.js#mount` | 未读红点 span |
| `.mcw-iframe` | `iframe-panel.js#create` | iframe 聊天窗 |

### 1.3 data 属性

- SDK 自身 `data-*` 全部使用 `kebab-case`：`data-app-key`、`data-channel-id`、`data-api-base-url`、`data-z-index`、`data-offset-x`、`data-offset-y`、`data-visitor-id-key`
- 在 `config.js#readDataAttrs` 中通过 `dataset` 读取时自动转为 `camelCase`：`d.appKey`、`d.channelId`、`d.apiBaseUrl`
- 新增 data 属性必须同步更新 README 配置表与本文档

### 1.4 事件回调命名

- 全部使用 `on` 前缀：`onReady`、`onOpen`、`onClose`、`onUnread`、`onMessage`
- 事件回调参数为对象，使用解构：`onUnread({ count })`、`onMessage({ type, payload })`、`onReady({ apiBaseURL, channelRef })`

### 1.5 postMessage 消息 type

- 全部使用 `mcw-` 前缀或 `chat-widget-` 前缀
- 现有 type：`mcw-config`、`mcw-ready`、`mcw-unread`、`mcw-message`、`chat-widget-close`
- 新增 type 必须在 [./ARCHITECTURE.md §4.1](./ARCHITECTURE.md) 中登记

---

## 二、文件规范

### 2.1 单一职责

每个 `src/*.js` 文件只承担一个明确职责：

| 文件 | 唯一职责 |
|---|---|
| `widget.js` | 入口编排（解析配置 + 组合浮标与面板 + 暴露 API） |
| `config.js` | 配置解析与合并 |
| `floating-button.js` | 浮标按钮 DOM 与交互 |
| `iframe-panel.js` | iframe 容器创建与 postMessage 通信 |
| `styles.css` | 全局样式（仅 hover/active/transition） |

### 2.2 入口文件 `widget.js` 不写业务逻辑

- `widget.js` 中的 `MarketingChatWidget` 类只做「组合」与「事件分发」
- 业务逻辑（DOM 操作、postMessage 收发、样式计算）下沉到 `FloatingButton` / `IframePanel`
- 用户回调统一通过 `_fireEvent(eventName, payload)` 触发，不在主流程中直接调用 `events.xxx`

### 2.3 文件头部 JSDoc

每个 `.js` 文件必须包含 `@file` 与 `@description`：

```js
/**
 * @file marketing-chat-widget 浮标 SDK 入口
 * @description HiveMtk 用户端 Chat Widget 嵌入 SDK(ADR-011)
 *
 * 用法(私域部署,最简集成):
 *   <script src="https://your-host/marketing-chat-widget.iife.js"></script>
 * ...
 */
```

### 2.4 类型定义

- 所有公开类与函数必须用 `@typedef` / `@param` / `@returns` 标注
- 单元测试 `test/unit.test.mjs` §7 会扫描所有源文件确保含 `@typedef` 与 `@param`
- 必须定义的类型：`McwConfig`、`McwEvents`、`FloatingButtonOptions`、`IframePanelOptions`（`test/unit.test.mjs` §7 强制校验）

---

## 三、样式规范

### 3.1 CSS 变量主题化

浮标主色通过 JavaScript 内联 `style.cssText` 注入（非 CSS 变量），保证可由 `config.color` 动态控制。`styles.css` 仅定义与颜色无关的动画：

```css
/* src/styles.css */
.mcw-floating-btn:hover {
  transform: scale(1.08);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.25);
}
.mcw-floating-btn:active {
  transform: scale(0.96);
}
.mcw-iframe {
  transition: opacity 0.2s;
}
```

### 3.2 选择器前缀

- 所有选择器必须以 `.mcw-` 开头
- **禁止**使用裸标签选择器（`div`、`iframe`、`body`）写 SDK 样式
- **禁止**使用 `*` 通配符
- **禁止**使用 `id` 选择器（避免与宿主冲突）

### 3.3 不使用 `!important`

- 全部样式通过 specificity 自然胜出
- 浮标关键样式走 `style.cssText` 内联（specificity 0,1,0,0），优先级足够
- 如遇宿主样式覆盖，应通过更高 specificity 解决，而非 `!important`

### 3.4 暗色模式

当前 SDK 默认浅色主题（白底 + `#1989fa` 主色），未实现暗色模式。如后续支持：

- 在 `config.js` 增加 `theme: 'light' | 'dark' | 'auto'` 字段
- 在 `floating-button.js#getStyle` 中根据 `theme` 切换 `color` 与 `background`
- iframe 内的暗色样式由 user-web SPA 自行实现，SDK 不干预

### 3.5 移动端样式

- `iframe-panel.js#getStyle` 检测 `window.innerWidth <= 480` 切换为全屏样式（`100vw` / `100vh`）
- 浮标本身在移动端保持 56×56 px，不缩放
- `setupVisualViewport` 监听 `window.visualViewport.resize/scroll` 跟随键盘弹起

---

## 四、配置规范

### 4.1 默认值集中管理

`src/config.js` 顶部 `DEFAULTS` 常量集中所有默认值：

```js
const DEFAULTS = {
  appKey: '',
  channelId: '',
  apiBaseURL: '',
  position: 'bottom-right',
  color: '#1989fa',
  title: '在线客服',
  welcome: '您好,请问有什么可以帮您?',
  lang: 'zh-CN',
  visitorIdKey: 'mtk_visitor_id',
  zIndex: 9999,
  offsetX: 24,
  offsetY: 24,
  width: 380,
  height: 560,
  allowedOrigins: null,
  events: {}
}
```

新增配置项必须：1）加入 `DEFAULTS`；2）在 `readDataAttrs` 中读取；3）在 `McwConfig` typedef 中标注；4）更新 README 配置表。

### 4.2 必填校验

- **没有任何字段是必填**（私域部署基线，ADR-011 §2.2）
- 缺失 `appKey` + `channelId` 时仅 `console.info` 提示，不报错（见 `widget.js#init`）
- 缺失 `apiBaseURL` 时 fallback 到 `resolveApiBaseURL(script)`（script 同源或 `window.location.origin`）

### 4.3 向后兼容

- 新字段一律可选，必须提供默认值
- 删除字段需提前 3 个 minor 版本 `console.warn` 弃用提示（ADR-011 §10.2）
- 主版本升级必须保持 `window.mcwInstance.open() / close() / destroy()` API 签名不变

### 4.4 解析优先级

固定顺序（见 `config.js#parseConfig`）：

```
DEFAULTS  <  readDataAttrs()  <  readQueryParams()  <  window.MarketingChatWidgetConfig
```

> 实际代码中合并顺序为 `DEFAULTS → readDataAttrs → readQueryParams → window.MarketingChatWidgetConfig`（后 `Object.assign` 覆盖前者），因此 `window.MarketingChatWidgetConfig` 优先级最高。文档以代码为准。

---

## 五、通信规范

### 5.1 postMessage 协议格式

所有 postMessage 消息体必须是对象，包含 `type` 与可选 `payload`：

```ts
interface McwMessage {
  type: string         // 必填，需在 ARCHITECTURE.md §4.1 登记
  payload?: any        // 可选
  count?: number       // 仅 mcw-unread 使用
  traceId?: string     // 可选，用于追踪（保留字段，当前未启用）
}
```

### 5.2 父端 → iframe

| type | payload | 时机 |
|---|---|---|
| `mcw-config` | `{appKey, channelId, channelRef, apiBaseURL, color, title, welcome, lang, visitorIdKey}` | `IframePanel.show()` 后 100ms 发送 |

发送时 `targetOrigin` 必须为 `new URL(this.apiBaseURL).origin`，**禁止 `'*'`**。

### 5.3 iframe → 父端

| type | payload | 时机 |
|---|---|---|
| `mcw-ready` | - | iframe 内 SPA 初始化完成 |
| `mcw-unread` | `{count: number}` | 未读数变化 |
| `mcw-message` | 业务消息体 | 收到新消息（透传给宿主 `onMessage`） |
| `chat-widget-close` | - | 用户在 iframe 内点击关闭按钮 |

### 5.4 WebSocket 消息格式

WebSocket 协议由 user-web SPA 与 user-server 直接约定，**embed-sdk 不参与**。建议格式：

```ts
{
  seq: number,        // 消息序号（客户端递增）
  ack?: number,       // ack 确认的 seq
  event: string,      // 事件类型：'message' | 'typing' | 'read' | 'presence'
  payload?: any
}
```

### 5.5 消息处理容错

- 接收消息时先校验 `data && typeof data === 'object'`（见 `widget.js#_onWindowMessage`、`iframe-panel.js#create`）
- 用户回调异常通过 `try/catch` 兜底，仅 `console.error`，不中断后续处理（见 `widget.js#_fireEvent`，由 `test/boundary.test.mjs` §6 覆盖）

---

## 六、安全规范

### 6.1 iframe sandbox 属性

当前 iframe 未启用 `sandbox`，仅设置 `allow="clipboard-write"`。如需启用：

```js
iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-forms allow-popups')
```

需评估对 user-web SPA 的 WebSocket、文件上传、`localStorage` 的影响。

### 6.2 CORS

- embed-sdk 自身不发起跨域 HTTP 请求（业务请求由 iframe 内 SPA 发起）
- user-server 通过 `CORS_ALLOW_ORIGINS_USER` 环境变量配置允许的 origin
- 静态资源（`/embed/*.js`）天然无 CORS 限制

### 6.3 XSS 防护

- **绝不**将 `postMessage` 收到的 `payload` 直接 `innerHTML` 到宿主 DOM
- 浮标 `innerHTML` 仅写入硬编码的 SVG（`ICON_SVG` / `CLOSE_SVG`）与未读数 `String(count)`（已是数字）
- `iframe.src` 由 `apiBaseURL` + `channelRef` 拼接，`channelRef` 通过 `encodeURIComponent` 编码
- 用户回调内如需操作 DOM，由用户自行做 HTML 转义

### 6.4 不存储敏感信息

- SDK 仅在 `localStorage` 中存储访客 UUID（key 由 `config.visitorIdKey` 控制，默认 `mtk_visitor_id`）
- **绝不**存储 AppKey、Token、用户身份等敏感信息到 `localStorage` / `sessionStorage` / `cookie`
- `console.*` 输出不包含任何 payload 明文（仅输出 `[MarketingChatWidget]` 前缀的状态信息）

### 6.5 origin 白名单

- `allowedOrigins` 严格匹配（含协议、端口、大小写）
- 自动推导逻辑：`[new URL(apiBaseURL).origin, window.location.origin]` 去重
- 测试覆盖：`test/boundary.test.mjs` §1（非法 origin）+ §13（危险协议）

---

## 七、兼容性规范

### 7.1 ES 版本

- 构建目标 `es2018`（`vite.config.js#build.target`）
- 源码可使用：`class`、模板字符串、`async/await`、可选链 `?.`、解构、`...` 展开
- **禁止**使用 ES2021+ 特性：`??=`、`||=`、`Promise.any`、`WeakRef`

### 7.2 不依赖 polyfill

- 不引入 `core-js`、`babel-polyfill`、`regenerator-runtime`
- 依赖浏览器原生：`URL`、`URLSearchParams`、`postMessage`、`localStorage`、`visualViewport`、`WebSocket`（仅 user-web SPA 使用）
- IE 11 不支持，文档明示（见 [./ARCHITECTURE.md §五](./ARCHITECTURE.md)）

### 7.3 移动端适配

- `window.innerWidth <= 480` 切换全屏 iframe
- `window.visualViewport` 监听键盘弹起（`setupVisualViewport`）
- 浮标位置 `bottom-right` / `bottom-left` 在移动端仍生效，但被全屏 iframe 覆盖

---

## 八、体积规范

- 构建产物 gzipped **< 30KB**（README §✨「轻量级」）
- 当前 `devDependencies` 仅 `vite` 一个，**禁止**增加运行时依赖
- 新增功能时优先考虑「iframe 内 user-web SPA 实现」而非「SDK 内实现」
- SVG 图标使用内联字符串（`ICON_SVG` / `CLOSE_SVG`），不引入图标库
- 定期检查 `dist/*.js` 大小，超过 50KB（uncompressed）需评估精简

---

## 九、禁止清单

| 禁止项 | 原因 |
|---|---|
| 依赖前端框架（Vue / React / Angular / Svelte） | SDK 必须零运行时依赖 |
| 修改宿主 DOM（除挂载浮标与 iframe 外） | 避免污染宿主页面 |
| 内联样式表（`<style>` 标签注入宿主） | 应通过 `style.cssText` 或 `styles.css` |
| 文件名带版本后缀（如 `widget.v1.2.0.js`） | 版本号在 `package.json` 管理，构建产物名固定 |
| 使用 `!important` | 通过 specificity 解决 |
| 使用 `eval` / `new Function` | CSP 不友好 |
| 使用 `'*'` 作为 postMessage targetOrigin | 安全风险 |
| 在 `widget.js` 写业务逻辑 | 入口文件只做编排 |
| 存储敏感信息到 localStorage | 安全风险 |
| 引入 polyfill 库 | 增加体积，与现代浏览器目标冲突 |
| 使用裸标签选择器（`div`、`body`）写样式 | 污染宿主 |
| 在 SDK 内实现 WebSocket 客户端 | 由 iframe 内 user-web SPA 负责 |
| 修改 `window.mcwInstance` / `window.MarketingChatWidget` 公开 API 签名 | 破坏向后兼容 |

---

最近更新日期: 2026-07-26
