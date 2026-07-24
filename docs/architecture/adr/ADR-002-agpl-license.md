# ADR-002: 开源协议统一为 AGPL-3.0-or-later

- **状态**：已采纳
- **日期**：2026-07
- **范围**：hivemtk + hivemtk-platform 全仓库

## 背景

平台端 `LICENSE` 文件是 MIT，但 `NOTICE` 文件声明 AGPL-3.0；两份法律文件互相矛盾，
对贡献者和下游使用方造成合规风险。

## 决策

- 主协议统一为 **GNU Affero General Public License v3.0 or later (AGPL-3.0-or-later)**
- 所有 Go 源文件首部添加 SPDX 标识（脚本批量注入）
- 移除"申请/在线客服账号/下载/定价"等商业化相关文案
- 不再提供 OTA 客户端更新通道

## 落地

- `hivemtk-platform/LICENSE` 改为 AGPL-3.0
- `hivemtk-platform/CONTRIBUTING.md` 移除"用户端 OTA 发布"段落
- 官网 `DocsPage.vue` 改为"无需注册、克隆即可私有化部署"
- README 移除"下载"字样

## 影响

- 商业集成必须以 AGPL-3.0 协议发布衍生作品
- 私有修改 + 公网部署的合规义务更明确
- 与上游 `vercel/next.js` / `element-plus` 等 AGPL 生态组件兼容
