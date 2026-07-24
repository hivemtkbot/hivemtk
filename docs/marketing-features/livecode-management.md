# 活码管理 (Live Code Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `livecode-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 活码管理 |
| 功能名称（英文） | Live Code / Dynamic QR Code |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | livecode |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（live_codes / live_code_targets / live_code_stats）
- [x] 后端 Service 与 Controller
- [x] 多目标 URL 动态分配
- [x] 分配策略：轮询/随机/加权
- [x] 二维码生成 + 统计
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

活码（动态二维码）解决微信群/个人号二维码 7 天过期问题。一个二维码扫描后动态跳转到不同的目标链接（实际客服/群）。

### 2.2 解决思路

- 多个目标 URL 组成目标列表
- 分配策略：RR（轮询）/ Random / Weighted（权重）
- 扫码 → 策略引擎选择目标 → 跳转
- 统计：每个目标的访问量

### 2.3 关键算法或模型

- 轮询：Redis INCR 计数器取模
- 随机：Redis SRANDMEMBER
- 加权：累计权重区间随机

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 活码名 |
| 输入 | targets | []object | 是 | 目标列表（URL + 权重） |
| 输入 | strategy | string | 是 | rr/random/weighted |
| 输出 | live_code_id | int64 | 是 | 活码ID |
| 输出 | qrcode_url | string | 是 | 二维码 URL |
| 输出 | short_url | string | 是 | 短链 URL |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/livecode | 列表 |
| POST | /api/livecode | 创建 |
| GET | /api/livecode/:id | 详情 |
| PUT | /api/livecode/:id | 更新 |
| DELETE | /api/livecode/:id | 删除 |
| GET | /api/livecode/:id/stats | 统计 |
| GET | /l/:code | 扫码跳转（公开） |
| POST | /api/livecode/:id/qrcode | 重新生成二维码 |

### 3.3 安全与合规

- 目标 URL 黑名单
- 访问限流
- 内容审核

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 扫码跳转 | < 50ms | ~15ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/livecode.go` | 接口 |
| Service | `internal/service/livecode_service.go` | 业务 |
| Repository | `internal/repository/livecode_repo.go` | 数据 |
| Model | `internal/model/live_code.go` | 模型 |
| Infra | Redis + OSS | 缓存+二维码存储 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| short-link | 短链 |
| obs-config | 二维码存储 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点 |

### 4.4 数据流向

```text
[商户] → 创建活码
   → [livecode_service.Create]
   → 写 live_codes + live_code_targets
   → 生成短链 → 写 short_links
   → 生成二维码 → 上传 OBS
   → 返回 qrcode_url

[访客] → 扫码 → 短链 → 活码
   → 策略引擎选择目标
   → 302 跳转
   → 写 live_code_stats
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建活码
2. 添加目标 URL（可设置权重）
3. 选择分配策略
4. 生成二维码
5. 下载/分享
6. 查看统计

### 5.2 系统处理流程

1. 鉴权
2. 参数校验
3. 写库
4. 生成短链 + 二维码
5. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 目标为空 | 400101 | 拒绝 |
| 权重和为 0 | 400102 | 拒绝 |

---

## 六、数据库设计

### 6.1 核心表 live_codes

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 活码名 |
| short_code | varchar(16) | UNIQUE | 短码 |
| strategy | varchar(16) | 非空 | rr/random/weighted |
| qrcode_url | varchar(512) | | 二维码 URL |
| click_count | int | 默认 0 | 总点击数 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

### 6.2 核心表 live_code_targets

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| live_code_id | bigint | FK | 活码 |
| url | text | 非空 | 目标 URL |
| weight | int | 默认 1 | 权重 |
| click_count | int | 默认 0 | 点击数 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

### 6.3 核心表 live_code_stats

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| live_code_id | bigint | FK | 活码 |
| target_id | bigint | FK | 目标 |
| stat_date | date | 非空 | 日期 |
| pv | int | 默认 0 | PV |
| uv | int | 默认 0 | UV |
| device | varchar(16) | | 设备 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建活码 | 完整参数 | live_code_id | ✅ |
| TC-002 | 轮询分配 | 100 次扫码 | 平均分配 | ✅ |
| TC-003 | 加权分配 | 权重 7:3 | 比例正确 | ✅ |
| TC-004 | 统计查询 | 活码 ID | 统计数据 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_TARGETS_PER_CODE | MAX_TARGETS_PER_CODE | 50 |

---

## 九、参考资料

- [shortlink-management.md](shortlink-management.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
