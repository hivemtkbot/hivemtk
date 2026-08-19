# HiveBridge 蜂桥 — 构建与发布文档

> 本扩展为**纯 JavaScript / Manifest V3** Chrome 扩展，
> 将抖音 / 小红书 / TikTok / 闲鱼 / 快手 网页私信桥接到 `user-server`（HiveMTK AI 智能体）。
> 本文档说明产品命名、Logo、本机构建与发布流程（无需 CI，macOS / Linux 直接命令执行）。

## 1. 产品命名与定位

| 项 | 值 |
|---|---|
| 产品名 | **HiveBridge 蜂桥** |
| 包名（内部） | `hivemtk-bridge` |
| 定位 | 多平台私信 ↔ AI 智能体 实时桥接（上行触发 AI / 下行回写网页 / 历史回填 / 拟人限速） |
| 适用平台 | 抖音 `douyin.com`、小红书 `xiaohongshu.com`、TikTok `tiktok.com`、闲鱼 `goofish.com`、快手 `kuaishou.com`（网页版） |
| 协议 | HTTP 三通道（`/api/bridge/ingest` 上行 / `/api/bridge/outbox` 下行 / `/api/bridge/outbox/ack` 状态） |

命名理由：项目代号 `hivemtk`（Hive Marketing），`Hive` 取「蜂巢」之意；桥接（Bridge）是核心能力，中文名「蜂桥」呼应蜂巢 + 连接。

## 2. Logo

- 源文件：`assets/logo.svg`（蜂巢六边形 + 蓝紫桥接弧线 + HiveBridge 字样）。
- 配色：琥珀金（蜂巢 `#FFB020→#FE7A00`）+ 科技蓝紫（桥接 `#4F8CFF→#7C5CFF`）。
- 已内联进 `src/popup/index.html`，popup 无需额外资源即可显示；
- 发布包亦包含 `assets/logo.svg` 供文档 / 官网使用。

## 3. 本机构建（开发态）

```bash
cd hivemtk/user-web/bridge
npm install
npm run build      # esbuild 逐入口独立 IIFE 打包 -> dist/
npm test           # vitest 单测（协议对齐 / 限速器 / HTTP 三通道 / 适配器 / 解析）
npm run dev        # watch 模式，改动即重打包
```

构建产物（`dist/`）：

| 文件 | 说明 |
| --- | --- |
| `manifest.json` | MV3 清单 |
| `background.js` | 后台 service worker（HTTP 三通道调度 + port 路由） |
| `content-douyin.js` / `content-xhs.js` / `content-tiktok.js` / `content-xianyu.js` / `content-kuaishou.js` | 各平台 content script |
| `popup.html` + `popup.js` | 配置 / 状态 / 自检弹窗 |
| `icons/icon-*.png` | 栅格图标 |

> 为何用 esbuild 而非 vite 多入口：vite 多入口会产生共享 chunk，导致 MV3 content script 因 `import` 外部 chunk 加载失败；esbuild 逐入口 IIFE 每个产物自包含，更稳定。

## 4. 本机发布（产出可分发 zip）

```bash
npm run release
```

流程（`scripts/release.mjs`，纯本机、无外部依赖）：

1. `npm run build` 生产构建；
2. 打包 `dist/` → `release/hivebridge-<version>.zip`（zip 根即 `manifest.json`，符合 Chrome 加载要求）；
3. 生成 `release/RELEASE_NOTES.md`（版本 / 能力 / 安装步骤 / 验证清单）。

> 版本号取自 `package.json` 的 `version`；发布前用 `npm version patch|minor|major` 递增。

## 5. 安装与加载

1. 打开 `chrome://extensions`，开启「开发者模式」；
2. 「加载已解压的扩展程序」→ 选择 `dist/`（或解压后的 `release/hivebridge-<version>.zip`）；
3. 点击工具栏图标 → 填 `user-server` 地址（如 `http://localhost:8204`）→ 保存；
4. 打开任一平台**私信页** → 点 popup 内「自检当前私信页」按 [./bridge.md §10](./bridge.md) 校准选择器。

## 6. 真机校准清单（上线前必做）

- [ ] 抖音：私信列表 / 消息气泡 / 输入框 / 发送按钮真实生效
- [ ] 小红书：`.xhs-im-conv-item` / `.chat-item` / `[contenteditable]` 输入框真实生效
- [ ] TikTok：DraftEditor 输入框 / 发送动作（Enter vs 飞机按钮）/ 消息气泡
- [ ] 闲鱼 / 快手：会话列表 / 消息气泡 / 输入框真实生效
- [ ] `account_id` 派生规则（平台账号标识稳定可取）
- [ ] 受控输入框填值不被框架拦截（粘贴 + input 事件双写生效）
- [ ] 会话切换时历史回填进入 `user-server`（`history` 帧，不触发 AI）
- [ ] 拟人节奏不触发平台风控

详见 [./bridge.md §10](./bridge.md) 与 [./docs/dev/DEVELOPMENT.md §8.4](./docs/dev/DEVELOPMENT.md)。
