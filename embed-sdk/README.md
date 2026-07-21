# marketing-chat-widget

marketing-tools-kit 客服系统的 Web Widget 嵌入 SDK（ADR-011）。

## 一行集成

将以下代码嵌入到企业网站 `</body>` 之前：

```html
<script
  src="https://your-cdn.com/marketing-chat-widget.iife.js"
  data-app-key="ak_xxxxxxxxxxxxxxxxxxxxxxxx"  <!-- 私域部署可省略，省略则用 default 渠道 -->
  data-color="#1989fa"
></script>
```

刷新页面即可看到右下角浮标。

## 配置参数（data-* 属性）

| 属性 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `data-app-key` | ❌ | - | 渠道 AppKey（从 B 端"客服渠道"管理获取）；**私域部署可省略**，省略时自动用 `default` 渠道（ADR-011：AppKey 软解析） |
| `data-api-base-url` | ❌ | 同源 | API 基础 URL，如 `https://api.example.com` |
| `data-position` | ❌ | `bottom-right` | `bottom-right` \| `bottom-left` |
| `data-color` | ❌ | `#1989fa` | 浮标主色（hex） |
| `data-title` | ❌ | `在线客服` | 聊天窗标题 |
| `data-z-index` | ❌ | `9999` | 浮标层级 |
| `data-offset-x` | ❌ | `24` | 水平边距（px） |
| `data-offset-y` | ❌ | `24` | 垂直边距（px） |

## 编程式控制

```javascript
// 打开聊天窗
window.mcwInstance.open()

// 关闭聊天窗
window.mcwInstance.close()

// 完全销毁
window.mcwInstance.destroy()
```

## 高级配置（全局变量）

```html
<script>
  window.MarketingChatWidgetConfig = {
    appKey: 'ak_xxx',
    apiBaseURL: 'https://api.example.com',
    color: '#ff6b35',
    position: 'bottom-left'
  }
</script>
<script src="https://your-cdn.com/marketing-chat-widget.iife.js"></script>
```

## 构建

```bash
npm install
npm run build  # 输出到 dist/
```

## 本地开发

```bash
npm run dev    # 启动 Vite dev server (http://localhost:5174/demo.html)
```
