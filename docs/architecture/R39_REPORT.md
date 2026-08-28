# R39 六步闭环报告（2026-08-29）

> 采纳标准（用户钦定）：**站在用户角度，功能是否需要，是否有必要 — 其他不考虑**

## 六步执行摘要

### Step1 功能清单+架构图
- 产出 `docs/architecture/FEATURE_INVENTORY_R39.md`：mermaid 架构图 + 20 域功能清单（A1-A21/B1-B6）+ 缺口清单
- 关键修正：`API_PAGE_INVENTORY.md` 的"209 断链"已过时（R32-38 修复了大量端点）

### Step2 网络调研（3 并行代理）
- 话术版本：Langfuse 整数版本+label 指针（非 semver）；Gong 曝光→成交归因；GrowthBook 分桶
- Feature Flags：Unleash/Flagsmith 管理端最小完备集（CRUD/rollout/audit/eval-log/stale）
- 知识库工作台：Dify/RAGFlow 核心集（文档状态/进度/chunk/Playground）
- RAG 评测：RAGAS/Langfuse golden set+run+diff；AB 统计：bayesian 胜率/SRM/CUPED/sequential
- 安全面板：authentik 事件审计+HIBP；邮件送达：Listmonk bounce 分桶

### Step3+4 论证+头脑风暴
- 产出 `docs/architecture/TASKS_R39.md`：K1-K16 论证表 + 红队二次论证（10 条质疑裁决）
- 运行时精确匹配（1462 条后端路由 dump × 670 前端调用）：**真实断链仅 50 条**（旧文档 209 条中 159 条已修复/死亡）

### Step5 一次性开发（12 域 · 48 端点 · 9 张新表）

| 域 | 交付 |
|----|------|
| T-6/T-7 话术版本+AB 曝光 | script_versions/script_exposure_logs 表；版本快照/激活/过期拦截；FNV 分桶+曝光 fire-and-forget+48h 归因窗转化回写+分桶转化率 API；/api/scripts/sync-to-library 接线 |
| K2 Feature Flags | feature_flags/audit/eval_logs 三表；CRUD+enable/disable+rollout+FNV evaluate+批量评估+审计+评估日志+stale 检测（14 端点） |
| K5 AB 高级统计 | bayesian 蒙特卡洛胜率/SPRT 序贯/SRM 卡方诊断/最小样本/多曝光/CUPED 方差缩减（6 端点，纯函数全单测） |
| CSAT 真实域 | csat_surveys 表；触发/评分/统计/趋势/差评/模板（7 端点，替换 R36 空态桩） |
| 客服协作 | 编辑锁（TTL 5min）/内部备注（is_internal）/坐席状态板/标签规则（关键词条件自动建 tag）/快捷回复文件夹+排序 |
| K14 knowledge 补齐 | 按 KB 上传/url/notion/feishu/dingtalk/crm 导入适配+document-types 枚举 |
| domain-pool 动作 | check/rotate/suspend/blacklist 查询/alerts+resolve（复检恢复语义） |
| 其余零散 | /api/ai/sales-cockpit 聚合、cards/cross-publish 跨平台发布（部分成功语义）、customer-events/batch、monitor/web-vitals、marketing-flows sync-ab-results、mentions read/mine 接真实通知、system/create-default-admin |
| 前端修复 | platformCard.js 路径错位修复（/api/${platform}Card → kebab-case 后端实际路由） |

### Step6 测试+修复（API+UI+数据+日志）

**构建/单测**：后端全仓 build+vet 绿；新增单测 20 例全绿（ScriptAB×4/FeatureFlag×3/AB统计×5/既有 Objection/DNC 回归）；user-web vitest 174/174；vite build 通过

**真实 HTTP 全链路验证（登录态）**：
- Feature Flags：创建→evaluate(payload 透传)→rollout→eval-log→审计→disable→删除 ✅
- T-7 闭环：曝光（bucket B）→DB 落行→转化回写→转化率 1.0 ✅（SQL 三表实证）
- CSAT：trigger→submit(2 分)→stats(avg=2)→negative(1)→trend(1)→template ✅
- 协作：edit-lock 抢/查/释、internal-notes 落库(is_internal=true)、agent-status-board(2 坐席)、标签规则自动建 tag+apply 命中（"深度测试"关键词→matched 标签）✅
- 知识：document-types 10 类型；驾驶舱聚合（react=1098 调用/llmRoutes=10）✅
- 其余：web-vitals 落库、customer-events/batch 2/2、cross-publish 1/1、marketing sync 空绑定优雅返回、domain-pool blacklist 查询 ✅

**发现并修复 3 个真 bug**：
1. `feature_flags.GetByKey` 反引号→PG 双引号（evaluate 恒 not_found）
2. gin 通配符冲突 `/domain-pool/:domain` vs `/:id` → 统一 `:id`
3. `domain_blacklist`/`domain_health_log` 表未注册迁移 → 补注册

**UI（Playwright）**：登录→9 路由遍历 8/9 PASS；唯一 WARN 为 /api/platform/message/latest 502（平台端未启动的本地环境预存项，非 R39 引入）

**预存 flaky（未触碰）**：TestHumanize_Particle/PlatformStyle 全量跑偶发、隔离跑恒绿（service 包全局态串扰，BACKLOG P2 已记录）

## 交付物清单
- 代码：user-server 25 文件（9 新建五层域文件+14 修改）、user-web 1 文件
- 文档：FEATURE_INVENTORY_R39.md / TASKS_R39.md / 本报告
- 测试：script_ab_test.go / feature_flag_test.go / ab_stats_test.go（20 例）

## 遗留（记录不阻塞）
- knowledge 连接器（notion/feishu/dingtalk/crm）凭据化对接由外部导入 job 承载（端点已给出明确契约）
- FeatureFlag CodeReferences 返回结构化空列表（本地自建无源码扫描器，注入点已留）
- humanize flaky 专项（预存）
