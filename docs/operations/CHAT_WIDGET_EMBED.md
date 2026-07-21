# HivemTK 用户端 - 嵌入式 Chat Widget 集成指南

> 私域部署下，企业如何在自己的官网嵌入客服浮标
> 适用版本：2026-07-21
> 关联 ADR：ADR-011（嵌入式客服 Chat Widget）

---

## 一、定位

企业有自己的官方网站，希望在右下角加一个**浮标客服入口**。点击后弹出聊天窗，与 HivemTK 用户端对接。

**几行代码**即可嵌入，无需复杂的对接开发。

---

## 二、架构

```
┌──────────────────────────────────────────────────┐
│             企业官网（如 www.example.com）            │
│                                                  │
│  <!-- 一行 script 即可嵌入 -->                       │
│  <script src="https://chat.example.com/embed/     │
│              marketing-chat-widget.iife.js"        │
│          data-app-key="optional"                  │
│          data-position="bottom-right"             │
│          data-color="#1989fa"                     │
│          data-welcome="您好，有什么可以帮您？">    │
│  </script>                                        │
│                                                  │
│       ┌──────────────────────┐                    │
│       │ 浮标 + iframe 聊天窗  │                    │
│       │  - 右下角悬浮按钮     │                    │
│       │  - 极简白底风格       │                    │
│       │  - max-width 380px   │                    │
│       └──────────┬───────────┘                    │
│                  │                                │
└──────────────────┼────────────────────────────────┘
                   │ HTTPS / WebSocket
                   ▼
   ┌──────────────────────────────────┐
   │  user-server (HivemTK 用户端)    │
   │  - RESTful API: /api/chat/public/*│
   │  - WebSocket: /api/ws/visitor    │
   │  - AI 自动回复（LLM + RAG）       │
   │  - 触发转人工后接管               │
   └──────────────────────────────────┘
```

---

## 三、关键决策（ADR-011 摘要）

### 1. 部署模式：私域独立部署

- 每个企业独立部署一套完整系统（user-server + user-web + PostgreSQL）
- 数据完全隔离
- **严禁** SaaS / 多租户模式，无 `merchant_id` 字段

### 2. AppKey 机制：可选

- AppKey/ChannelID 不是必填
- 缺失时使用默认 `default` 渠道
- 中间件改为**软解析**（缺失时跳过鉴权）

### 3. WebSocket：无鉴权

- 私域部署，WebSocket 通道全部是企业自己的网站
- `/api/ws/visitor` 端点无强制鉴权

### 4. UI：极简白底

- 主色：白底 + 蓝色强调（#1989fa）
- 不使用阴影、渐变等装饰
- 移动端：max-width 380px

### 5. 附件：访客直传七牛

- 后端仅下发上传凭证，访客浏览器直传七牛 CDN
- 文件类型白名单：image / file / audio / video
- 单文件 ≤ 20MB
- Token 有效期：1 小时

### 6. 转人工：NLP 关键词自动触发

- 前端**不暴露**"转人工"按钮（永远显示"在线客服"）
- 后端基于关键词（"人工"/"真人"/"转人工"/"human"/"operator"/"客服"/"agent"/"找人"）自动触发
- 命中后跳过 AI 推理，直接走自动分配

---

## 四、嵌入步骤

### 4.1 准备浮标脚本

`embed-sdk` 是 HivemTK 提供的网页客服浮标 SDK（独立前端工程），构建后产物在 `embed-sdk/dist/`。

由 `user-server` 在 `/embed/` 路径下服务（通过 docker-compose 挂载 `embed-sdk/dist` 到容器内的 `/app/embed-sdk-dist`）。

### 4.2 嵌入企业官网

```html
<!-- 在企业官网 </body> 前加入 -->
<script src="https://chat.example.com/embed/marketing-chat-widget.iife.js"
        data-app-key="optional-channel-id"
        data-position="bottom-right"
        data-color="#1989fa"
        data-welcome="您好，有什么可以帮您？">
</script>
```

### 4.3 必传参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `src` | 浮标脚本 URL | 必填 |
| `data-app-key` | 渠道 ID（可选）| `default` |
| `data-position` | 浮标位置 | `bottom-right` |
| `data-color` | 主色 | `#1989fa` |
| `data-welcome` | 欢迎语 | 默认值 |

---

## 五、用户端域名要求

- 浮标脚本 URL 域名 **必须** 与 `user-server` 部署域名一致
- WebSocket 走同一域名（由 SDK 自动推断）
- nginx 反代需配置 `/api/chat/public/*` 与 `/api/ws/visitor` 路径

---

## 六、嵌入 SDK 自动行为

- 页面加载完成后自动注入右下角浮标按钮
- 用户点击后弹出 iframe 聊天窗（最大宽度 380px）
- iframe 内容由 user-server 提供（`/chat/embed/:app_key` 路径）
- 父子页面通过 `postMessage` 通信
- 访客身份由 localStorage 生成的 UUID 跟踪

---

## 七、聊天窗功能

| 功能 | 说明 |
|------|------|
| 实时消息 | WebSocket 长连接 |
| 消息历史 | localStorage 缓存 + API 拉取 |
| 快捷回复 | 内置常见问题模板 |
| 文件上传 | 七牛直传，支持图片/文档/音频/视频 |
| 转人工 | 关键词触发（前端不显示按钮）|
| 离线消息 | 客服不在线时留言，下次访客打开仍可见 |

---

## 八、SDK 集成模式

### 8.1 自动注入（默认）

页面加载后自动注入浮标，无需额外配置。

### 8.2 自定义容器

在页面放一个 `<div id="mtk-chat"></div>`，SDK 会自动识别并把浮标挂载到该容器内（适合需要自定义位置的场景）。

---

## 九、移动端适配

- 聊天窗 max-width: 380px
- 浮标位置避免与浏览器工具栏重叠
- 自动检测移动设备尺寸

---

## 十、安全

- 访客消息 HTML 转义（防 XSS）
- WebSocket 无鉴权（私域部署，访客是企业自己的用户）
- 速率限制：30 条消息/分钟/IP
- iframe 通信用 postMessage + origin 校验

---

## 十一、CDN 部署

为提高全球访问速度，可把 `embed-sdk/dist/` 单独部署到 CDN：

```bash
# embed-sdk 构建后
npm run build

# 上传到 CDN
aws s3 sync dist/ s3://cdn.example.com/embed/ \
  --cache-control "public, max-age=31536000"
```

企业官网改为引用 CDN：

```html
<script src="https://cdn.example.com/embed/marketing-chat-widget.iife.js" ...>
</script>
```

---

## 十二、相关代码

- `embed-sdk/src/` - SDK 源代码
- `user-server/internal/router/chat_routes.go` - `/api/chat/public/*` 路由
- `user-server/internal/websocket/handler.go` - `/api/ws/visitor` 端点
- `user-server/internal/service/visitor_chat.go` - 访客聊天服务
- `migrations/004_customer_session.sql` - customer_sessions 表（platform=web_embed）

---

## 十三、相关文档

- 完整 ADR：[adr/ADR-011-chat-widget-embed.md](adr/ADR-011-chat-widget-embed.md)
- RAG 自动回复架构：[RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md](RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md)
- 部署架构：[部署方案_用户端.md](部署方案_用户端.md)
