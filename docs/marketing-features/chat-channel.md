# 客服 Web Widget 渠道管理 (Chat Channel)

> **所属模块**: customer-service
> **功能 slug**: `chatChannel`
> **文档定位**: 商户创建多个 Web Widget 渠道（每个渠道一个 channel_ref/app_key），嵌入到不同网站；管理渠道配置（样式/欢迎语/路由智能体）。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 客服 Web Widget 渠道管理 |
| 功能名称(英文) | Chat Channel |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 多 Web Widget 渠道创建（每个渠道独立 channel_ref/app_key）
- [x] 渠道配置（样式/欢迎语/路由智能体）
- [x] 嵌入安装指南（InstallGuide）
- [x] allowed_origins 域名白名单校验
- [x] `setupChatChannelAdminRoutes` + `internal/controller/chat_channel_controller.go`
- [x] 前端列表、创建、编辑、安装指南页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] Widget 主题模板市场
- [ ] 多语言欢迎语配置

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
商户拥有多个网站，每个网站需要独立的客服 Widget（不同样式、不同欢迎语、不同路由智能体）。通过多 Widget 渠道管理，实现一商户多站点独立客服部署。

### 2.3 关键算法或模型
- 渠道身份认证（app_key 签名校验）
- 域名白名单匹配（allowed_origins）
- 智能体路由（按 agent_id 分发）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 渠道名称 |
| 输入 | agent_id | int64 | 是 | 路由智能体 |
| 输入 | widget_config | object | 是 | Widget 配置 |
| 输入 | allowed_origins | array | 是 | 域名白名单 |
| 输出 | channel_id | int64 | 是 | 渠道 ID |
| 输出 | channel_ref | string | 是 | 渠道引用 |
| 输出 | app_key | string | 是 | 应用密钥 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- Widget 加载 < 500ms
- 渠道配置生效 < 1s
- 单商户渠道上限 50

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/chat-channels | 渠道列表 | JWT |
| POST | /api/chat-channels | 创建渠道 | JWT |
| GET | /api/chat-channels/:id | 渠道详情 | JWT |
| PUT | /api/chat-channels/:id | 更新渠道 | JWT |
| DELETE | /api/chat-channels/:id | 删除渠道 | JWT |
| GET | /api/chat-channels/:id/install-guide | 安装指南 | JWT |
| POST | /api/chat-channels/widget/auth | Widget 认证（公开） | app_key |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| chat_channels | Web Widget 渠道 |
| chat_channel_widgets | Widget 配置 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| channel_id | bigint | 渠道 ID |
| channel_ref | varchar(64) | 渠道引用 |
| app_key | varchar(128) | 应用密钥 |
| agent_id | bigint | 路由智能体 |
| widget_config | jsonb | Widget 配置 |
| allowed_origins | jsonb | 域名白名单 |

---

## 六、业务流程
### 6.1 主流程
1. 商户创建 Web Widget 渠道
2. 配置样式、欢迎语、路由智能体
3. 设置 allowed_origins 域名白名单
4. 获取 channel_ref 与 app_key
5. 查看安装指南，复制嵌入代码到网站
6. 网站访客打开 Widget → app_key 认证 → 域名校验 → 路由智能体

### 6.2 异常处理
- app_key 无效：返回 401，拒绝 Widget 加载
- 域名不在白名单：返回 403，提示配置 allowed_origins
- 智能体不可用：降级为离线留言模式

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 渠道列表 | /chatChannel/list | chatChannel/List.vue |
| 创建渠道 | /chatChannel/create | chatChannel/Create.vue |
| 编辑渠道 | /chatChannel/edit/:id | chatChannel/Edit.vue |
| 安装指南 | /chatChannel/install/:id | chatChannel/InstallGuide.vue |

### 7.2 关键交互
- 列表展示渠道状态与最近活跃时间
- 创建页支持样式实时预览
- 安装指南提供嵌入代码一键复制
- allowed_origins 支持批量添加与校验

---

## 八、测试策略
### 8.1 单元测试
- 渠道 CRUD service 单测
- app_key 签名校验单测
- allowed_origins 域名匹配单测

### 8.2 集成测试
- 创建渠道→嵌入代码→Widget 认证全链路
- 域名白名单拦截验证
- 智能体路由分发验证
