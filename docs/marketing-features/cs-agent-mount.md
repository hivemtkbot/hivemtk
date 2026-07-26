# 客服座席挂载智能体 (Customer Service Agent Mount)

> **所属模块**: multi-ai-agent
> **功能 slug**: `csAgentMount`
> **文档定位**: 客服座席可挂载智能体作为辅助，AI 自动建议回复、自动接管夜间会话。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 客服座席挂载智能体 |
| 功能名称(英文) | Customer Service Agent Mount |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | multi-ai-agent |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 客服座席与智能体挂载关系 CRUD
- [x] 自动接管开关（auto_takeover）
- [x] 工作时段配置（working_hours）
- [x] 夜间/非工作时段自动接管会话
- [x] AI 自动建议回复（座席辅助模式）
- [x] `CustomerServiceAgentController` + `service.NewCustomerServiceAgentService`
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 座席接管指标看板（接管率/响应时长）
- [ ] 智能体接管质量评分

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
客服座席在高峰期需要 AI 辅助建议回复，在夜间/非工作时段需要 AI 自动接管会话以保持服务连续性。通过座席挂载智能体，实现人机协同与无人值守切换。

### 2.3 关键算法或模型
- 工作时段判定（按周/按日/时段）
- 接管切换状态机（seat_active / agent_takeover）
- 建议回复生成（基于上下文 + RAG + SOP）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | agent_id | int64 | 是 | 智能体 ID |
| 输入 | seat_id | int64 | 是 | 客服座席 ID |
| 输入 | auto_takeover | bool | 否 | 是否自动接管 |
| 输入 | working_hours | object | 否 | 工作时段 |
| 输入 | status | string | 是 | 状态 |
| 输出 | id | int64 | 是 | 挂载关系 ID |
| 输出 | suggest_reply | string | 否 | 建议回复 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 建议回复生成 < 2s (P95)
- 接管切换延迟 < 1s
- 单座席挂载智能体上限 1（主备可扩展）

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/cs-agent-mounts | 挂载关系列表 | JWT |
| POST | /api/cs-agent-mounts | 创建挂载 | JWT |
| PUT | /api/cs-agent-mounts/:id | 更新挂载 | JWT |
| DELETE | /api/cs-agent-mounts/:id | 删除挂载 | JWT |
| POST | /api/cs-agent-mounts/:id/takeover | 手动触发接管 | JWT |
| POST | /api/cs-agent-mounts/:id/suggest | 生成建议回复 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| cs_agent_mounts | 客服座席与智能体挂载关系 |
| cs_agent_takeover_logs | 接管日志 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 主键 |
| agent_id | bigint | 智能体 ID |
| seat_id | bigint | 客服座席 ID |
| auto_takeover | bool | 是否自动接管 |
| working_hours | jsonb | 工作时段 |
| status | varchar(16) | 状态 |

---

## 六、业务流程
### 6.1 主流程
1. 管理员进入客服座席管理页
2. 选择座席并挂载智能体
3. 配置工作时段与 auto_takeover
4. 工作时段内：消息进入→智能体生成建议→座席确认/修改→发送
5. 非工作时段：消息进入→智能体自动接管→直接回复
6. 座席上班后可手动接管回会话

### 6.2 异常处理
- 智能体不可用：降级为座席手动处理，记录告警
- 建议回复超时：座席手动输入，超时不上报
- 接管冲突：同一会话只允许一个接管方，先到先得

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 座席挂载列表 | /csAgentMount/list | csAgentMount/List.vue |
| 座席工作台（建议回复面板） | /csWorkspace | csWorkspace/Index.vue |

### 7.2 关键交互
- 工作时段配置支持可视化时间网格
- 建议回复面板支持一键采纳/编辑/拒绝
- 接管状态实时显示（座席在线/智能体接管中）
- 手动接管按钮带二次确认

---

## 八、测试策略
### 8.1 单元测试
- 挂载关系 CRUD service 单测
- 工作时段判定单测（跨日/跨周）
- 接管切换状态机单测

### 8.2 集成测试
- 工作时段内建议回复全链路
- 非工作时段自动接管全链路
- 座席手动接管回会话验证
