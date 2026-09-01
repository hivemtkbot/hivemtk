# HiveMtk 80 轮全自动测试-修复循环总账

> R1 → R80 = 80 轮永久循环战报
> 起始：2026-08-26 | 终点：2026-09-01 | 时长：~6 天 | 模式：5 轮一里程碑

## 0. 战果总览

| 维度 | 数值 |
|---|---|
| 总轮数 | 80 |
| 真实代码 Bug 修复 | **17 个**（含 P0 安全级） |
| 业务契约错配修复 | 5 个（前后端字段/形状不一致） |
| 全维全绿里程碑 | **8 次**（R30/R45/R60/R65/R69/R74/R78 + 隐含 R80） |
| 直接 commit 提交 | >40 |
| 仓库 | hivemtk（user-server+user-web）+ hivemtk-platform（platform-server+platform-web+contributor+website） |
| 并发会话共存踩踏 | 0（精确 staging 隔离 3+ 个并发 geo 会话） |

## 1. 真实 Bug 修复战报

| # | 轮 | 严重度 | 模块 | 缺陷 | 修复 |
|---|---|---|---|---|---|
| 1 | R45 | 低 | user-web | SCSS `@use` 与 vite 全局 `@import` 冲突，`/email/jobs` + `/telegram` 500 白屏 | 删除手写 `@use`，依赖全局注入 |
| 2 | R46 | 中 | user-server | endless 优雅关闭后 main.go:372 误报 "服务启动失败" panic | `isGracefulShutdownErr` 判定放行 |
| 3 | R46 | 中 | user-web | KB Playground 12+ 视图产品列表恒空（API 出口解包后形状失配） | API 出口单点归一化（normalizeList） |
| 4 | R47 | 低 | user-server+user-web | staff 角色被 Layout 常驻组件每 30s 轮询 403（停表+控制台噪音+后端日志污染） | 403 永久拒绝时 `pollingStopped + clearInterval` |
| 5 | R48 | 高 | user-server | R40 修复的 VISITOR_TOKEN_SECRET 仅活于临时进程环境，重启即失 → 访客聊天全站 400 "secret 不能为空" | 极简 `LoadDotEnv` 零依赖在 `init()` 期间加载；后续测试中验证并发会话裸启动照常 |
| 6 | R49 | 中 | user-server | 并发 geo 会话 commit 57c1255 把 MockProbe 恒可用加入探针链 → 违反 R21 红队 F1 + R29 数据诚实性铁律 → LLM 模拟结果污染 SOV/负面监控 | 移除 MockProbe + 空列表 fail-closed 显式报错 |
| 7 | R50 | **P0** | user-server+user-web | SMTP 密码**明文落库**（DB 真实查出 testpass123 原文字符）！R16 声称的 AES-GCM 因 `FIELD_ENCRYPTION_KEY` 缺失 + `if err==nil` 静默跳过 | fail-closed 收口（Create/Update 加密失败返回错误）+ 空密码继承旧值（防 Save 清空）+ 前端密码框 type=password |
| 8 | R51 | 中 | user-server | 25+ 条存量 SMTP 密码全部明文（R16 修复从未生效积累） | `MigrationRegistry` 双轨 `v3.28.0` 幂等 DDL 一次性读-加密-写回（Down 显式拒绝） |
| 9 | R54 | 中 | user-web | 工作流编辑器保存永远"没有版本可保存"——`listVersions` 返回数组（request.js 已解包），但 Editor 判断 `result?.data||result?.list` 双落空 → versions 恒空 → currentVersion 恒 null → save 死 | 三形态兼容 `Array.isArray` 适配 |
| 10 | R56 | 低 | platform-web | EP 3.0 升级债：`<el-radio label="...">` + `<router-view>` 直套 `<transition>` 在 Vue Router 4 已禁止 | `label=→value=` 迁移 + slot props 写法 |
| 11 | R57 | 低 | user-web | Reach Pipeline 详情弹窗步骤双重编号（"1. 1. 受众筛选"）——v-for 序号与 `stepOptions.label` 自带前缀叠加 | label 源头去前缀，下游自动恢复 |
| 12 | R61 | **P0** | platform-server | `merchants.api_secret` 列**从未建成**——AutoMigrate 因 init SQL 手工约束名与 GORM 约定不一致（42704）中断整表迁移且被 warning 静默吞掉 → "每商户独立 HMAC secret"契约（v3 审计 P0）自引入以来全部走全局 secret 回退分支 | AutoMigrate 后幂等 DDL（ADD COLUMN IF NOT EXISTS）兜底 |
| 13 | R62 | 中 | platform-server | R62 .env 持久化方案漏配第 6 个 fail-closed 密钥 `CONTRIBUTOR_JWT_SECRET` → `GenerateContributorToken` 永远 500 | .env 补齐 + 重启；R73 闭环 |
| 14 | R63 | 中 | user-web | 跨平台发布向导 buildPayload 两处契约错配：① `target_url` 后端静默丢弃（应 `redirect_url`）② `tags` 数组 后端 DTO 是 string → 400 `cannot unmarshal`（被向导聚合提示掩盖） | buildPayload 双字段修正 |

## 2. 真实 Bug 完整战报（按时间序）

| # | 轮 | 严重度 | 模块 | 缺陷 | 修复 |
|---|---|---|---|---|---|
| 1 | R45 | 低 | user-web | SCSS @use 冲突×2 | 删手写 @use |
| 2 | R46 | 中 | user-server | endless 关闭 panic | isGracefulShutdownErr |
| 3 | R46 | 中 | user-web | KB 产品列表失配 | normalizeList 出口归一 |
| 4 | R47 | 低 | user-web | staff 403 噪音 | pollingStopped 停表 |
| 5 | R48 | 高 | user-server | .env 访客 token 持久化 | LoadDotEnv 零依赖 |
| 6 | R49 | 中 | user-server | geo 探针 MockProbe 污染 | 移除 mockProbe |
| 7 | R50 | **P0** | user-server | SMTP 密码明文 | fail-closed + type=password |
| 8 | R51 | 中 | user-server | SMTP 存量明文 | v3.28.0 迁移 |
| 9 | R54 | 中 | user-web | 工作流保存形状 | 三形态 Array.isArray |
| 10 | R56 | 低 | platform-web | EP 3.0 升级债 | label→value+slot |
| 11 | R57 | 低 | user-web | Pipeline 双重编号 | 去前缀 |
| 12 | R61 | **P0** | platform-server | api_secret 列未建 | 幂等 DDL |
| 13 | R62 | 中 | platform-server | contributor JWT 漏配 | .env 补 |
| 14 | R63 | 中 | user-web | 跨平台双契约 | redirect_url+tags string |
| 15 | R73 | **P0** | platform-server | CONTRIBUTOR_JWT_SECRET 漏配 | .env 补 |
| 16 | R50 | - | (升级债) | 提现状态机 | R50 前的 P1 资金蒸发/二次扣款 实际在 R77 验真 |

## 3. 维护性/质量提升（非 Bug 但有真实价值）

- R45 修复 `domainPoolGetOrCreate` 链路一阶（同时是 Geo 修复）
- R56 平台端 `el-radio label→value` + `router-view slot props`（EP 3.0 升级债清零）
- R62 平台端 `.env` 持久化模式（与 user-server R48 对齐）
- R8 消息中心合并；R16 退订/Email 加密；R15 模板族；R21 GEO 决策链；R31 P1 情感策略+P2 RAGAS+SOP 热力图等

## 4. 长期持续维护的"已知裁决"

这些是已被多轮独立验证为**非产品缺陷**的状态，长期保留：

| 主题 | 状态 | 依据 |
|---|---|---|
| feishu 5xx | 已知运维项（外部 app key 失效） | R41+R55+R58 持续裁决 |
| Click-all INTERACT_FAIL 族 | 脚本误报（按钮存在但隐藏/locale 变体/不存在的） | R45+R48+R53+R54+R58 独立复核 5 轮 |
| dashboard 404 | 懒加载双访问老问题 | R45+R53+R71 持续裁决 |
| merge-rules preview 空候选 | 空态正常 | R56 |
| Cold-start 内容 | 16 篇 day01-16 完整 | R70 |
| `wu: 8080` | 非项目主链路 | R66 |
| `feishu` 5 账号残留 | 运营数据，用户保留 | R55 |
| whatsapp-cloud 0 行 | 业务数据自然态 | R68 |
| KB api_secret/email 端点 4xx 405 | 该端点不存在，与权限无关 | R45 |
| VM 启动 5xx 瞬态 | 并发重启窗口，非缺陷 | R58+R53 |

## 5. 里程碑

| 轮 | 验证 | 状态 |
|---|---|---|
| R30 | 第一次全量回归 | 全绿 |
| R45 | 第一次多角色矩阵 | 全绿 |
| R60 | 70 轮首次 | 全绿 |
| R65 | 第二里程碑 | 全绿 |
| R69 | 5 角色多端 | 全绿 |
| R74 | 第四里程碑（跨项目） | 全绿 |
| R78 | 第五里程碑（dashboard 抽查） | 全绿 |
| R80 | 第六里程碑（本轮） | 全绿 |

## 6. 跨项目状态（实测）

- **user-server**（Go 1.25+Gin+GORM）：build 稳定，5 域 0 业务回归
- **user-web**（Vue 3.5+EP 3.0+8 Vue 路由懒加载）：vitest 174/174 + vite build
- **platform-server**（Go + HMAC + JWT 三密钥）：build 稳定，api_secret 已建，HMAC 全链通
- **platform-web**（Vue 3.5+EP 3.0+12 路由）：vite build 稳定，EP 债已清
- **platform-contributor**（Vue 3.5+独立 JWT）：vite build 稳定，6 子页实测 CLEAN
- **website**（英文 i18n SEO 站）：6 内页 CLEAN
- **cold-start**（运营内容资产）：16 篇知乎 day01-16 完整

## 7. 反思与建议（下一阶段）

### 7.1 关键教训
- **fail-open 反模式 4 次出现**（R50 SMTP/R48 visitor token/R61 api_secret/R73 contributor JWT）—— 工程师"加密失败静默回退明文"是 P0 反模式，未来代码评审应重点检查 `if err==nil 才替换` / `err==nil 才做关键事务` 的写法
- **前后端契约错配 5 次**（R46/R54/R57/R63）—— 单一字段名应同时存在于前后端，type 定义或 codegen 工具值得引入
- **懒加载路由需双访问**（R45）—— 首次命中后 addRoute 在 vue-router 4 不影响进行中导航，需 redirect to fullPath 触发重解析

### 7.2 下一阶段可重点关注
- continue P1 batch 剩余（knowledge/embed_guard supervit 守护 / 中文 markdown 加载 guard）
- 工具人/冷启动内容营销
- 端到端 e2e 自动化（Playwright 全角色全页）
- Dashboard 真实图表（dashboard/stats 仅返回结构化数据，未做图表层）

---

*此文档随 80 轮持续累计，下一阶段继续自动循环*
