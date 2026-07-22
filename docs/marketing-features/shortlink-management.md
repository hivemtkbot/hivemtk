# 短链接管理 (Short Link Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `shortlink-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 短链接管理 |
| 功能名称（英文） | Short Link Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | shortlink |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（short_links / short_link_stats）
- [x] 后端 Service 与 Controller
- [x] 短码生成（6 位随机 + 去重）
- [x] 密码保护 + 过期时间
- [x] 访问统计（PV/UV/来源/地域）
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

营销推广需要短链接（替代长 URL）+ 数据统计。短链是卡片、邮件、短信等场景的标配。

### 2.2 解决思路

- 短码生成：6 位 Base62（a-zA-Z0-9）随机字符串 + 数据库去重
- 缓存：Redis SET short:{code} → 原始 URL，TTL=过期时间
- 跳转：访问短链 → 缓存查找 → 301/302 重定向
- 统计：访问时累加 + 写日志
- 密码保护：URL hash 对比

### 2.3 关键算法或模型

- 短码生成：`base62.RandomString(6)` + `Retry(3)`
- UV 计算：Redis HLL（HyperLogLog）按天聚合
- 来源识别：Referer Header 解析

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | long_url | string | 是 | 长链接 |
| 输入 | domain | string | 否 | 短链域名 |
| 输入 | custom_code | string | 否 | 自定义短码 |
| 输入 | password | string | 否 | 访问密码 |
| 输入 | expire_at | timestamp | 否 | 过期时间 |
| 输入 | title | string | 否 | 备注标题 |
| 输出 | short_code | string | 是 | 短码 |
| 输出 | short_url | string | 是 | 完整短链 |
| 输出 | qrcode | string | 是 | 二维码 URL |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/short-link | 短链列表 |
| POST | /api/short-link | 创建短链 |
| GET | /api/short-link/:code | 短链详情 |
| PUT | /api/short-link/:code | 更新短链 |
| DELETE | /api/short-link/:code | 删除短链 |
| GET | /api/short-link/:code/stats | 统计数据 |
| GET | /s/:code | 短链跳转（公开） |
| POST | /s/:code/verify | 密码验证 |

### 3.3 安全与合规

- 短链黑名单（不允许的域名）
- 密码 bcrypt 哈希
- 访问限流
- 内容审核（链接安全性）

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 短链跳转 | < 50ms | ~15ms |
| 短码生成 | < 50ms | ~10ms |
| 并发跳转 | 5000 QPS | 8000 QPS |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/shortlink.go` | 接口 |
| Service | `internal/service/shortlink_service.go` | 业务 |
| Repository | `internal/repository/shortlink_repo.go` | 数据 |
| Model | `internal/model/shortlink.go` | 模型 |
| Infra | Redis + LRU | 缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| domain-pool | 域名池 |
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| card-* | 卡片分享 |
| email-send | 邮件追踪 |
| 营销自动化 | 流程节点 |

### 4.4 数据流向

```text
[商户] → 创建短链
   → [shortlink_service.Create]
   → 生成 6 位短码 → DB 去重
   → 写 short_links
   → Redis 缓存 short:{code} → long_url
   → 返回 short_url + qrcode

[访客] → GET /s/:code
   → 查 Redis → 缓存命中
   → 验证密码（如果有）
   → 301 重定向 → 累加统计
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 输入长链接
2. 选择域名（来自域名池）
3. 可选：自定义短码、密码、过期
4. 创建
5. 复制短链或扫码
6. 查看统计数据

### 5.2 系统处理流程

1. 鉴权
2. 参数校验（URL 格式、域名白名单）
3. 短码生成/校验自定义
4. 写库
5. 缓存
6. 生成二维码
7. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| URL 非法 | 400101 | 拒绝 |
| 自定义短码已存在 | 409001 | 提示占用 |
| 域名失效 | 400102 | 提示换域名 |

---

## 六、数据库设计

### 6.1 核心表 short_links

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| short_code | varchar(16) | UNIQUE | 短码 |
| long_url | text | 非空 | 长链接 |
| domain | varchar(128) | 非空 | 短链域名 |
| title | varchar(256) | | 标题 |
| password_hash | varchar(255) | | 密码哈希 |
| expire_at | timestamp | | 过期时间 |
| click_count | int | 默认 0 | 点击数 |
| status | tinyint | 非空 | 0=禁用 1=启用 2=过期 |
| created_at | timestamp | 非空 | |

### 6.2 核心表 short_link_stats

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| short_code | varchar(16) | FK | 短码 |
| stat_date | date | 非空 | 日期 |
| pv | int | 默认 0 | PV |
| uv | int | 默认 0 | UV |
| referer | varchar(64) | | 来源 |
| device | varchar(16) | | 设备 |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_shortlink_code | short_code | UNIQUE | 短码唯一 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建短链 | 长 URL | short_code | ✅ |
| TC-002 | 自定义短码 | 6 位字符 | 200 | ✅ |
| TC-003 | 自定义冲突 | 重复 | 409001 | ✅ |
| TC-004 | 密码保护 | 设置密码 | 跳转前需验证 | ✅ |
| TC-005 | 访问统计 | 100 次点击 | PV=100 | ✅ |
| TC-006 | 过期短链 | 等过期 | 404001 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SHORT_CODE_LENGTH | SHORT_CODE_LENGTH | 6 |
| CACHE_TTL | CACHE_TTL | 3600 |

---

## 九、参考资料

- [FUNCTION_DETAILS.md 第三章](../architecture/FUNCTION_DETAILS.md#三营销模块---短链接管理)
- [domain-pool.md](domain-pool.md)
- [livecode-management.md](livecode-management.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
