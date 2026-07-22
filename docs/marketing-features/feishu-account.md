# 飞书账号管理 (Feishu Account)

> **所属模块**: community-management
> **功能 slug**: `feishu`
> **文档定位**: 飞书机器人账号管理，配合 reach.feishu.send 工具发送消息。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 飞书账号管理 |
| 功能名称(英文) | Feishu Account Management |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | community-management |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 飞书机器人账号管理（app_id / app_secret 加密存储）
- [x] tenant_access_token 自动获取与刷新
- [x] `setupFeishuRoutes` 路由注册
- [x] `internal/controller/feishu_account_controller.go` 后端控制器
- [x] 配合 `reach.feishu.send` 工具向飞书用户发送消息
- [x] 账号有效性检测与 token 过期预警
- [x] 前端 `user-web/src/views/feishu/FeishuAccount.vue` 账号管理

### 1.2 待完成内容
- [ ] 飞书群组管理增强

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
飞书是重要的企业协同平台，私域销售需要通过飞书机器人向客户或内部团队推送消息、卡片、任务提醒。系统需集中管理飞书应用凭证，自动维护 token 生命周期，供 reach 工具调用。

### 2.2 解决思路
在系统中注册飞书应用（app_id + app_secret），secret 加密存储；定时调用飞书开放接口获取并缓存 tenant_access_token；reach.feishu.send 工具调用时携带 token 发送消息；token 临期自动刷新，失败告警。

### 2.3 关键算法或模型
- 凭证加密：AES-256-GCM 加密 app_secret
- token 刷新：提前 5 分钟刷新（token 有效期 2 小时）
- 失败重试：发送失败时刷新 token 后重试 1 次

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | app_id | string | 是 | 飞书应用 ID |
| 输入 | app_secret | string | 是 | 飞书应用密钥 |
| 输入 | name | string | 是 | 账号名称 |
| 输出 | account_id | int64 | 是 | 账号 ID |
| 输出 | tenant_access_token | string | 是 | 租户访问令牌 |
| 输出 | valid_until | timestamp | 是 | token 失效时间 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- token 获取 < 500ms（缓存命中 < 10ms）
- 消息发送 < 1s
- token 刷新成功率 ≥ 99.9%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/feishu/account/list | 账号列表 | JWT |
| POST | /api/feishu/account | 新建账号 | JWT |
| PUT | /api/feishu/account/:id | 更新账号 | JWT |
| DELETE | /api/feishu/account/:id | 删除账号 | JWT |
| POST | /api/feishu/account/:id/refresh-token | 手动刷新 token | JWT |
| POST | /api/feishu/account/:id/test | 发送测试消息 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| feishu_accounts | 飞书账号主表 |
| feishu_token_cache | token 缓存表 |
| feishu_send_logs | 发送日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| app_id | varchar(64) | 飞书应用 ID |
| app_secret | varchar(255) | 加密后的应用密钥 |
| tenant_access_token | varchar(255) | 租户访问令牌 |
| valid_until | timestamp | token 失效时间 |
| name | varchar(64) | 账号名称 |

---

## 六、业务流程
### 6.1 主流程
1. 运营人员在账号管理页注册飞书应用凭证
2. 系统加密存储 app_secret
3. 首次调用时获取 tenant_access_token 并缓存
4. reach.feishu.send 工具调用时读取缓存 token 发送消息
5. token 临期前 5 分钟自动刷新
6. 发送失败时刷新 token 重试 1 次

### 6.2 异常处理
- token 获取失败：记录日志，告警，reach 工具返回错误
- app_secret 错误：账号标记为无效，提示重新填写
- 发送限流：指数退避重试

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 飞书账号管理 | /feishu/account | feishu/FeishuAccount.vue |

### 7.2 关键交互
- 账号列表表格（名称、app_id、token 状态、失效时间）
- 新增/编辑账号表单（app_id、app_secret 密文输入）
- 手动刷新 token 按钮
- 发送测试消息弹窗
- token 即将失效高亮提示

---

## 八、测试策略
### 8.1 单元测试
- app_secret 加密/解密单测
- token 刷新时机判断单测
- 失败重试逻辑单测

### 8.2 集成测试
- token 获取与缓存测试
- reach.feishu.send 工具端到端测试
- token 失效后自动恢复测试

---

## 九、版本历史
| 版本 | 日期 | 变更说明 |
|---|---|---|
| v1.0 | 2026-07-15 | 初始实现 |
| v1.1 | 2026-07-22 | 补充功能文档 |

---

## 十、相关文档
- [../INDEX.md](../INDEX.md)
- [../architecture/ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- [../CROSS_COMPARISON_REPORT.md](../CROSS_COMPARISON_REPORT.md)
