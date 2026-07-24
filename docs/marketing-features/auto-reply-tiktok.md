# TikTok 自动回复 (TikTok Auto Reply)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `auto-reply-tiktok`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | TikTok 自动回复 |
| 功能名称（英文） | TikTok Auto Reply |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | auto-reply |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构
- [x] 后端 Service 与 Controller
- [x] TikTok 专用规则模板
- [x] chromedp 多语言适配
- [x] 多语言自动回复
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

TikTok 海外多市场，私信咨询需用当地语言回复。需支持英语/日语/韩语/泰语/印尼语等多语种。

### 2.2 解决思路

- 复用 chromedp 引擎
- 多语言关键词字典（每语言独立规则集）
- 翻译 fallback：未匹配时调用 LLM 翻译后匹配

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_id | int64 | 是 | TikTok 账号ID |
| 输入 | message | string | 是 | 收到的消息（原始语言） |
| 输入 | lang | string | 是 | 自动检测语言 |
| 输出 | reply | string | 是 | 回复内容（同语言） |
| 输出 | translated | bool | 是 | 是否经过翻译 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/tiktok-auto-reply/accounts | 账号 |
| POST | /api/tiktok-auto-reply/accounts | 添加 |
| GET | /api/tiktok-auto-reply/rules | 规则 |
| POST | /api/tiktok-auto-reply/rules | 创建 |
| GET | /api/tiktok-auto-reply/logs | 日志 |
| PUT | /api/tiktok-auto-reply/status | 启停 |

### 3.3 安全与合规

- Cookie 加密
- 多语言内容审核
- 速率限制

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 消息响应延迟 | < 5s | ~3.5s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/tiktok_auto_reply.go` | 接口 |
| Service | `internal/service/tiktok_auto_reply_service.go` | 业务 |
| Repository | `internal/repository/tiktok_auto_reply_repo.go` | 数据 |
| Model | `internal/model/tiktok_auto_reply.go` | 模型 |
| Infra | `internal/browser/chromedp.go` + Redis | 浏览器+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| card-tiktok | TikTok 卡片 |
| LLM Dispatcher | 翻译能力 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点 |

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加 TikTok 账号
2. 选择语言
3. 创建规则（关键词+回复）
4. 启动服务
5. 查看日志

### 5.2 系统处理流程

1. chromedp 接收消息
2. 语言检测
3. 关键词匹配（同语言）
4. 未命中 → 翻译 → 再匹配
5. 拼装回复（同语言）
6. chromedp 发送
7. 写日志

---

## 六、数据库设计

### 6.1 核心表 tiktok_auto_reply_rules

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| lang | varchar(8) | 非空 | 语言代码 |
| keywords | jsonb | 非空 | 关键词 |
| reply_content | text | 非空 | 回复内容 |
| priority | int | 默认 0 | 优先级 |
| status | tinyint | 非空 | 状态 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 日语回复 | "いくら?" | 日语回复 | ✅ |
| TC-002 | 英语回复 | "How much?" | 英文回复 | ✅ |
| TC-003 | 翻译兜底 | 未知关键词 | LLM 翻译后匹配 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SUPPORTED_LANGS | SUPPORTED_LANGS | en,ja,ko,th,id,vi |

---

## 九、参考资料

- [auto-reply-universal.md](auto-reply-universal.md)
- [card-tiktok.md](card-tiktok.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
