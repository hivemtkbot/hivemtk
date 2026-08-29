# R40 闭环报告（2026-08-29 · 第二轮循环）

> 采纳标准（用户钦定）：**站在用户角度，功能是否需要，是否有必要 — 其他不考虑**

## 六步执行摘要

### Step1 复扫
- 后端路由 1462→1528 条（含 R39 交付）；前端断链 50→**0**（3 条为模板字符串解析伪影/注释死代码）

### Step2 调研
- 复用 R39 三代理调研结论 + 补充：连接器测试连接（Notion/飞书/钉钉官方探测端点口径）

### Step3+4 论证
- R39 遗留与 TECH_DEBT 逐项核实：**M2/M4/M5/M7/M8/M11/M12 已在前几轮修复**；M1 已修（WeComQuotaResetCron 每日 00:05 已装配）；M3 判定**无需修**（outreach 流水每条独立 ID 语义正确，B-4 教训反证内容哈希是错的）
- R40 任务定案 5 项

### Step5 开发
| # | 任务 | 交付 |
|---|------|------|
| R40-1 | TestHumanize flaky 根治 | 根因=`maybeInjectTypo` 全局 rand 10% 概率改写"好的"→"好哒"污染断言；HumanizePolisher 注入 `randFn` 随机源，17 处断言型测试统一注入确定性函数；**count=20 全绿**（原 count=5 即挂） |
| R40-2 | 知识连接器凭据管理 | `kb_connectors.go` 五层：GET/PUT /api/knowledge/connectors（4 源）+ POST test 真实连通探测（Notion Bearer/飞书 tenant_token/钉钉 gettoken/CRM webhook）；读取侧密钥脱敏（尾4位）；测试结果回写 |
| R40-3 | FeatureFlag code-references | 占位空列表→KV 登记表实现：GET 查询 + POST 工具链登记 |
| R40-4 | platform/message/latest 优雅空态 | 单机部署（无平台端）轮询 502 错误流→静默空消息 200（仅此轮询端点，不影响其余平台代理语义） |
| R40-5 | ZOMBIE_API ③ 处置 | /domainpool/delete|check/:id 旧蛇形路由删除（前端全走 /api/domain-pool/*，grep 无脚本依赖）；mfa/verify 与 v1/asset-market 确认为扫描误报保留 |

### Step6 测试+修复
- 构建+vet 绿（修 1 处 self-assignment）
- 运行时验证：connectors 4 源 listed、notion 保存脱敏(****wxyz)、**feishu/dingtalk 真实连接成功(HTTP 200)**、notion 假 token 诚实报 401、CRM webhook 404 如实上报；code-references 登记→查询闭环；platform/message/latest 200 空态；旧蛇形路由 404；R39 端点回归抽查全过
- 回归：service（ScriptAB/FeatureFlag/Humanize/Objection/DNC ×5 轮）+ ops 全包 全绿

## 已提交
commit 见 git log（feat(r40)）→ Gitee + GitHub 双远端

## 遗留
- connectors 的凭据仅供人工/后续拉取器消费（文档导入执行仍走 url/content 管线——连接器自动拉取属后续迭代）
- ZOMBIE ② 类 ~200 条按原则"不删代码"，移交产品排期（本次未动）
