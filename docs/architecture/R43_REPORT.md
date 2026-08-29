# R43 全功能综合测试报告（2026-08-29 · 第五轮循环）

> 采纳标准（用户钦定）：**站在用户角度，功能是否需要，是否有必要 — 其他不考虑**
> 测试方法：四维交叉验证（日志 + API 全量 + UI 点击遍历 + DB 一致性）→ 发现问题立即修复

## 测试计划与执行

### T1 日志分析（user-server 运行日志全量解析）
- ERROR 11 条 / WARN 28 条分类定位
- 发现 1 个真实缺陷：**TG 假 bot token 401 无限重试**（demo 数据 3 个 bot，polling worker 指数退避永不放弃）

### T2 API 全量扫描（1532 路由 dump × 带认证验证）
- 无参 GET 535 个全扫：**533 OK / 0 超时 / 0 HTML异常 / 2 业务错误**
- 2 个错误均为平台代理端点"平台配置未初始化"（message/list、user/list）——单机部署预期语义，管理端操作型报错正确，不静默
- 带参 GET 抽样 10 组（真实 ID）：全部符合预期（含 404 语义正确的空表）
- 排查确认：`GET /api/customer-sessions/:id` 数字 ID 语义为后端独有端点，前端全部走字符串 session_id 路径，非缺陷

### T3 UI 模拟人工点击遍历（142 路由 × 交互抽样）
- **142/142 全 PASS**（零 console 错误 / 零非 401 网络错误 / 零页面异常）
- 交互抽样：每页点击"新增/创建/刷新"类首枚按钮 + 对话框关闭 + Escape

### T4 DB 一致性核查（R39-R42 全部写入链路）
- script_versions=1、script_exposure_logs=1（T-7 闭环）、feature_flags=2、flag_eval_logs=4、code_refs=1
- csat_surveys=1、quick_reply_folders=1、web_vital_records=1、tag_rules=2、internal_notes=1、connector_creds=4
- douyin_cards 脏外链=0（R42 清理生效）——**全部计数与操作历史吻合，零数据丢失/不一致**

### T5 发现问题立即修复
| 缺陷 | 修复 | 验证 |
|------|------|------|
| TG polling 401 无限重试（假 token demo bot 刷屏日志+无效外呼） | `isTelegramAuthError` 判定 401=确定性退出（与 409 同级语义），单次 ERROR 明确指引"渠道页更换 Token"；重启对账同理 | 重启后同错误从持续重试降为启动期各 1 次；polling worker 遇 401 单次告警即停 |

### 最终回归
- 重启后 UI 再遍历：142/142 PASS
- build+vet 绿、TG 相关单测绿（带测试库 env）

## 综合判断
系统当前四维全绿：API 533/535（2 个为预期平台代理错误语义）、UI 142/142、DB 零不一致、日志无循环错误。唯一真实缺陷（TG 401 重试）已修复。

## 已提交
commit 见 git log（fix(r43)）→ Gitee + GitHub 双远端

## 遗留（不阻塞）
- 平台代理 2 端点的"平台配置未初始化"——单机部署语义正确；有平台端部署时自然消除
- TG demo 假 bot 数据（111/222/333）——修复后不再产生日志噪音，数据保留供渠道页演示
