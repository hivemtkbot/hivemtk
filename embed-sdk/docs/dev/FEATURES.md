# embed-sdk 功能清单

> **规则级别**: ⭐⭐ 项目级开发文档

> 本清单枚举 embed-sdk 的全部功能特性、配置项、事件回调、安全特性与测试覆盖。
> 关联文档：[../../README.md](../../README.md)、[./ARCHITECTURE.md](./ARCHITECTURE.md)、[./DEVELOPMENT.md](./DEVELOPMENT.md)、[./CONVENTIONS.md](./CONVENTIONS.md)、[../../../docs/operations/CHAT_WIDGET_EMBED.md](../../../docs/operations/CHAT_WIDGET_EMBED.md)

---

## 一、浮窗按钮（FloatingButton）

| 功能 | 实现位置 | 说明 |
|---|---|---|
| 默认位置 | `floating-button.js#getStyle` | `bottom-right`（右下角） |
| 自定义位置 | `config.position` | 支持 `bottom-right` / `bottom-left` |
| 自定义颜色 | `config.color` | hex 值，默认 `#1989fa`，通过 `style.cssText` 注入 `background` |
| 自定义层级 | `config.zIndex` | 默认 `9999` |
| 自定义边距 | `config.offsetX` / `config.offsetY` | 默认 `24px` |
| 圆形浮标 | `getStyle` | 56×56 px，`border-radius: 50%`，box-shadow `0 4px 16px rgba(0,0,0,0.2)` |
| 客服气泡 SVG 图标 | `ICON_SVG` 常量 | 24×24 viewBox，`currentColor` 跟随 `color` |
| 关闭 SVG 图标 | `CLOSE_SVG` 常量 | 点击后切换为关闭图标（X） |
| 悬浮动画 | `styles.css .mcw-floating-btn:hover` | `transform: scale(1.08)` + 加深阴影 |
| 点击动画 | `styles.css .mcw-floating-btn:active` | `transform: scale(0.96)` |
| 状态切换 | `setOpen(opened)` | 切换 `mcw-open` class、切换 icon、清零未读 |
| 未读消息红点 | `setUnread(count)` | `.mcw-fab-badge`，`#f56c6c` 红色，`count > 99` 显示 `99+` |
| 红点显隐 | `setUnread` 内 `display: flex/none` | `count > 0` 显示，否则隐藏 |
| 无障碍标签 | `aria-label="打开在线客服"` | `mount()` 中 `setAttribute` |
| 防重复挂载 | `mount()` 内 `if (this.button) return` | 多次调用安全 |
| 编程式卸载 | `unmount()` | `button.remove()` + 置空 |

---

## 二、聊天面板（IframePanel）

| 功能 | 实现位置 | 说明 |
|---|---|---|
| iframe 内嵌 | `iframe-panel.js#create` | `document.createElement('iframe')`，`className = 'mcw-iframe'` |
| 独立路由 | `iframe.src` | `${apiBaseURL}/chat/embed/${channelRef}#/chat/embed/${channelRef}` |
| channel 路由 | `getChannelRef()` | 优先 `channelId`，其次 `appKey`，最后 `default` |
| channelRef 编码 | `encodeURIComponent(channelRef)` | 防止特殊字符破坏 URL |
| 自定义宽度 | `config.width` | 默认 `380px`，移动端自动 `100vw` |
| 自定义高度 | `config.height` | 默认 `560px`，移动端自动 `100vh` |
| 标题 | `iframe.title = config.title` | 默认「在线客服」，无障碍 |
| 圆角阴影 | `getStyle` | `border-radius: 10px`，`box-shadow: 0 4px 24px rgba(0,0,0,0.15)` |
| 层级 | `z-index = config.zIndex + 1` | 确保覆盖浮标 |
| 显隐切换 | `show()` / `hide()` | `display: block/none`，防重复 |
| 销毁 | `destroy()` | `iframe.remove()` + 移除 message 监听 + 清理 visualViewport |
| 剪贴板权限 | `iframe.allow = 'clipboard-write'` | 仅授予写入权限 |
| 配置注入 | `show()` 后 100ms `postMessage('mcw-config', ...)` | 透传 appKey/channelId/color/title/welcome/lang 等 |
| 关闭消息处理 | `messageHandler` 接收 `chat-widget-close` | 调用 `hide()` + `onClose()` 回调 |

### 聊天窗内功能（由 user-web SPA 提供）

> 以下功能在 iframe 内由 user-web SPA 实现，embed-sdk 仅负责透传配置与事件。

| 功能 | 说明 |
|---|---|
| 消息列表 | 历史消息 + 实时消息渲染 |
| 输入框 | 文本输入 + 发送 |
| 表情 | emoji 选择器 |
| 图片 | 上传 / 预览 / 七牛直传 |
| 文件 | 文档 / 音频 / 视频，七牛直传 |
| 快捷回复 | 内置常见问题模板 |
| 转人工 | 关键词触发（前端不显示按钮） |
| 离线消息 | 客服不在线时留言 |

---

## 三、实时通信（WebSocket）

| 功能 | 实现方 | 说明 |
|---|---|---|
| WebSocket 连接 | user-web SPA | `ws://apiBaseURL/api/ws/visitor?session_id=xxx&channel_id=xxx&visitor_id=xxx` |
| 握手参数校验 | user-server | 缺 `session_id` 返回 HTTP 400 业务校验错误（`test/ws.test.mjs` 验证） |
| 鉴权 | 无 | 私域部署模式下无强制鉴权（ADR-011 §2.3） |
| 自动重连 | user-web SPA | 断线后自动重连（iframe 内实现，SDK 不参与） |
| 心跳 | user-web SPA | 维持连接活跃 |
| ack | user-web SPA + user-server | 消息携带 `seq`，接收方回 `ack` |
| 离线消息补发 | user-web SPA | 重连后拉取 `GET /api/chat/public/sessions/{id}/offline-messages` 补发 |
| 限流 | user-server | 30 条消息/分钟/IP |
| 速率超限降级 | user-server | 超出后业务降级处理 |
| 端点测试 | `test/ws.test.mjs` | Node 22+ 内置 WebSocket 验证端点存活 |

> embed-sdk 本身不直连 WebSocket，所有实时通信由 iframe 内 user-web SPA 负责。

---

## 四、配置项

完整配置项总表（16 项）：

| # | 字段 | data-* 别名 | 默认值 | 必填 | 说明 |
|---|---|---|---|---|---|
| 1 | `appKey` | `data-app-key` | `''` | ❌ | 渠道 AppKey |
| 2 | `channelId` | `data-channel-id` | `''` | ❌ | 直接指定 channel_id |
| 3 | `apiBaseURL` | `data-api-base-url` | 同源 | ❌ | API 基础 URL |
| 4 | `position` | `data-position` | `bottom-right` | ❌ | 浮标位置 |
| 5 | `color` | `data-color` | `#1989fa` | ❌ | 浮标主色（hex） |
| 6 | `title` | `data-title` | `在线客服` | ❌ | 聊天窗标题 |
| 7 | `welcome` | `data-welcome` | `您好,请问有什么可以帮您?` | ❌ | 欢迎语 |
| 8 | `lang` | `data-lang` | `zh-CN` | ❌ | 语言 |
| 9 | `visitorIdKey` | `data-visitor-id-key` | `mtk_visitor_id` | ❌ | localStorage 访客 UUID key |
| 10 | `zIndex` | `data-z-index` | `9999` | ❌ | 层级 |
| 11 | `offsetX` | `data-offset-x` | `24` | ❌ | 水平边距 px |
| 12 | `offsetY` | `data-offset-y` | `24` | ❌ | 垂直边距 px |
| 13 | `width` | `data-width` | `380` | ❌ | 聊天窗宽度 |
| 14 | `height` | `data-height` | `560` | ❌ | 聊天窗高度 |
| 15 | `allowedOrigins` | - | 自动推导 | ❌ | postMessage origin 白名单 |
| 16 | `events` | - | `{}` | ❌ | 事件回调集合 |

### 解析优先级

```
window.MarketingChatWidgetConfig  >  query 参数  >  data-* 属性  >  DEFAULTS
```

### `allowedOrigins` 自动推导

```js
origins = [new URL(apiBaseURL).origin, window.location.origin] // 去重
```

---

## 五、事件回调

| # | 事件名 | 触发时机 | 回调参数 |
|---|---|---|---|
| 1 | `onReady` | SDK 初始化完成（`init()` 末尾） | `{apiBaseURL: string, channelRef: string}` |
| 2 | `onOpen` | 聊天窗打开（`open()` / `toggle(true)`） | 无 |
| 3 | `onClose` | 聊天窗关闭（`close()` / `toggle(false)` / iframe 触发 `chat-widget-close`） | 无 |
| 4 | `onUnread` | 收到 `mcw-unread` postMessage | `{count: number}` |
| 5 | `onMessage` | 收到任意非 `mcw-unread` 的 postMessage | `{type: string, payload: any}` |

### 注册方式

```js
window.MarketingChatWidgetConfig = {
  events: {
    onReady: function (info) { console.log('ready', info) },
    onOpen: function () { console.log('opened') },
    onClose: function () { console.log('closed') },
    onUnread: function ({ count }) { console.log('unread:', count) },
    onMessage: function ({ type, payload }) { console.log(type, payload) }
  }
}
```

### 异常容错

`widget.js#_fireEvent` 内 `try/catch`，回调抛错仅 `console.error`，不中断 SDK：

```js
try {
  fn(payload)
} catch (err) {
  console.error('[MarketingChatWidget] event ' + eventName + ' error:', err)
}
```

覆盖测试：`test/boundary.test.mjs` §6（5 个回调全部抛错，SDK 仍正常运行）。

---

## 六、多语言支持

| 字段 | 支持值 | 说明 |
|---|---|---|
| `lang` | `zh-CN`（默认） | 简体中文 |
| `lang` | `en-US` | English |

- `lang` 通过 `mcw-config` postMessage 透传给 iframe
- 实际多语言切换由 user-web SPA 内部 i18n 实现
- SDK 自身的 `console.*` 输出与默认 `title` / `welcome` 始终为中文（私域部署默认场景）

---

## 七、移动端适配

| 功能 | 实现位置 | 说明 |
|---|---|---|
| 浮窗全屏 | `iframe-panel.js#getStyle` | `window.innerWidth <= 480` 时切换为 `100vw` / `100vh` |
| 全屏样式 | `getStyle` | `top/left/right/bottom: 0`，无圆角，无阴影 |
| 键盘弹起处理 | `setupVisualViewport` | 监听 `window.visualViewport.resize/scroll` |
| 视口跟随 | `apply()` | 同步 `vv.offsetTop/offsetLeft/width/height` 到 iframe style |
| 清理监听 | `clearVisualViewport` | `destroy()` 时移除 resize/scroll 监听 |
| 浮标不缩放 | `floating-button.js` | 移动端浮标仍为 56×56 px |
| 触摸优化 | `user-select: none` | 浮标禁止文本选中 |

### visualViewport 兼容性

- iOS Safari 13+ 支持
- Android Chrome 61+ 支持
- 不支持时 `setupVisualViewport` 直接 return（无降级，但移动端仍可全屏使用）

---

## 八、安全特性

| 特性 | 实现位置 | 说明 |
|---|---|---|
| iframe sandbox | `iframe-panel.js#create` | 当前未启用 `sandbox` 属性，仅 `allow="clipboard-write"` |
| CSP 友好 | 全局 | 不使用 `eval` / `new Function`，无内联 `<style>` 注入宿主 |
| 跨域隔离 | `config.allowedOrigins` | 自动推导 `[apiBaseURL.origin, window.location.origin]` |
| origin 严格校验 | `widget.js#_bindMessageListener` + `iframe-panel.js#create` | 接收时 `allowedOrigins.includes(e.origin)` |
| 禁用 `'*'` targetOrigin | `iframe-panel.js#show` | `new URL(this.apiBaseURL).origin` |
| XSS 防护 | 全局 | SVG 硬编码、`encodeURIComponent(channelRef)`、不 innerHTML payload |
| 用户回调隔离 | `widget.js#_fireEvent` | `try/catch` 兜底，回调崩溃不影响 SDK |
| 不存储敏感信息 | 全局 | localStorage 仅存访客 UUID（`mtk_visitor_id`） |
| 私域部署无鉴权 | user-server | WebSocket `/api/ws/visitor` 无强制鉴权（ADR-011 §2.3） |
| 速率限制 | user-server | 30 条消息/分钟/IP |
| 危险协议防护 | `parseConfig` + `postMessage` | `new URL()` 解析失败时 fallback；危险 origin 不在白名单则静默 return |

---

## 九、集成方式

| 方式 | 文件 | 适用场景 |
|---|---|---|
| `<script>` 标签 | `dist/marketing-chat-widget.iife.js` | 最简集成，任意网站 |
| ES Module | `dist/marketing-chat-widget.esm.js` | npm 安装 / 现代打包工具 |
| npm 包 | `package.json#main` / `module` / `unpkg` / `jsdelivr` | `import MarketingChatWidget from 'marketing-chat-widget'` |
| 全局变量配置 | `window.MarketingChatWidgetConfig` | 无法修改 HTML 时 |
| query 参数 | `?app_key=&channel_id=&lang=` | URL 透传（优先级最低） |
| CDN 部署 | 静态资源走 CDN，API 走 user-server | 跨域场景 |
| FRP 穿透 | user-server 内网，云端 frps 暴露 | NAT 内网部署 |
| 同源部署 | `<script src="/embed/...">` | 最简，无 CORS |

### 编程式 API

```js
window.mcwInstance.open()      // 打开聊天窗
window.mcwInstance.close()     // 关闭聊天窗
window.mcwInstance.destroy()   // 销毁（移除 DOM + 清理监听）
window.mcwInstance.toggle(opened)  // 切换开关状态
window.mcwInstance.config      // 当前生效配置
window.MarketingChatWidget     // 构造函数（高级用法）
```

---

## 十、测试覆盖

| 测试类型 | 文件 | 章节数 | 运行命令 |
|---|---|---|---|
| 单元测试 | `test/unit.test.mjs` | 10 | `npm run test` |
| 边界测试 | `test/boundary.test.mjs` | 13 | `npm run test:boundary` |
| WebSocket 测试 | `test/ws.test.mjs` | 2 | `npm run test:ws` |
| 端到端测试 | `test/e2e.sh` | 11 | `npm run test:e2e` |
| 全量测试 | - | - | `npm run test:all` |

### 单元测试覆盖（`unit.test.mjs`）

1. SDK 加载与全局变量（`MarketingChatWidget` / `mcwInstance`）
2. 默认配置值（16 项全部断言）
3. FloatingButton 行为（mount / unmount / setOpen / setUnread）
4. IframePanel 行为（apiBaseURL / channelRef / allowedOrigins）
5. 跨域 origin 校验（非法 origin 拒绝）
6. 事件回调（onReady 触发一次）
7. JSDoc 类型定义完整性（4 个 typedef）
8. 关键关键字在源码中存在
9. demo.html 6 场景覆盖
10. docs 索引完整性

### 边界测试覆盖（`boundary.test.mjs`）

1. allowedOrigins 白名单边界（7 种非法 origin）
2. 消息体格式异常（8 种畸形 data）
3. 超大消息体（1MB / 10MB）
4. allowedOrigins 配置异常（5 种）
5. 多次 mount / destroy 循环
6. 用户事件回调异常不崩溃
7. URL 解析边界（6 种畸形 apiBaseURL）
8. config.js SSR 守卫完整性
9. query 参数非法值
10. 多次 init 不重复挂载
11. 多种 postMessage 消息类型（7 种）
12. 快速 open/close 循环（50 次）
13. 危险协议 origin 处理（`javascript:` / `data:` / `vbscript:`）

### e2e 测试覆盖（`e2e.sh`）

0. 前置检查（user-server 健康、SDK 产物、node）
1. user-server 关键端点
2. SDK 端点内容
3. CORS 跨域配置
4. WebSocket 端点存活
5. demo.html 6 场景 + 3 占位符
6. SDK 源码 JSDoc 完整性
7. 跨域 origin 校验逻辑
8. FRP 模板生成
9. 文档一致性
10. 单元测试
11. WebSocket 实测

---

## 十一、总计统计

### 核心模块数量

| 类别 | 数量 |
|---|---|
| 源码文件（`src/`） | 5（widget.js / config.js / floating-button.js / iframe-panel.js / styles.css） |
| 核心类 | 3（`MarketingChatWidget` / `FloatingButton` / `IframePanel`） |
| 核心函数 | 5（`parseConfig` / `readDataAttrs` / `readQueryParams` / `resolveApiBaseURL` / `DEFAULTS`） |
| 测试文件 | 4（unit / boundary / ws / e2e.sh） |
| JSDoc 类型定义 | 6（`McwConfig` / `McwEvents` / `McwPosition` / `McwMessage` / `FloatingButtonOptions` / `IframePanelOptions`） |
| 构建产物 | 2（IIFE + ESM） |

### 配置项数量

| 类别 | 数量 |
|---|---|
| 总配置项 | 16（见 §四） |
| 必填项 | 0（私域部署基线） |
| 可选项 | 16 |
| 通过 `data-*` 暴露 | 14（`allowedOrigins` 与 `events` 仅支持 `window.MarketingChatWidgetConfig`） |

### 事件回调数量

| 类别 | 数量 |
|---|---|
| 总回调数 | 5（`onReady` / `onOpen` / `onClose` / `onUnread` / `onMessage`） |
| 触发时机明确 | 5 |
| 异常容错覆盖 | 5（全部 `try/catch`） |

### postMessage 消息类型

| 方向 | 数量 | 类型 |
|---|---|---|
| parent → iframe | 1 | `mcw-config` |
| iframe → parent | 4 | `mcw-ready` / `mcw-unread` / `mcw-message` / `chat-widget-close` |
| 合计 | 5 | - |

### 集成方式数量

| 类别 | 数量 |
|---|---|
| 集成方式 | 8（`<script>` / ESM / npm / 全局变量 / query / CDN / FRP / 同源） |
| 编程式 API | 5（`open` / `close` / `destroy` / `toggle` / `config`） |

### 测试覆盖统计

| 测试类型 | 章节数 |
|---|---|
| 单元测试 | 10 |
| 边界测试 | 13 |
| WebSocket 测试 | 2 |
| e2e 测试 | 11 |
| **合计** | **36** |

### 体积统计

| 指标 | 目标 | 实际 |
|---|---|---|
| gzipped | < 30KB | 满足（README §✨） |
| uncompressed | < 50KB | 满足 |
| devDependencies | 1（`vite`） | 满足 |
| runtime dependencies | 0 | 满足 |

---

最近更新日期: 2026-07-26
