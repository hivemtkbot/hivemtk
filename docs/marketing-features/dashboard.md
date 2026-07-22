# 数据大屏 (Dashboard)

> **所属模块**: marketing-automation
> **功能 slug**: `dashboard`
> **文档定位**: 实时数据可视化大屏,支持公开分享,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 数据大屏 |
| 功能名称(英文) | Dashboard |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | marketing-automation |
| 优先级 | P1 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端拖拽式大屏设计器
- [x] 公开分享与权限控制
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 大屏模板市场

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户需要集中展示核心经营指标(销售额、新客、转化率、活跃用户),支持大屏投放(会议、展厅)与公开分享。

### 2.2 解决思路

大屏 = 多个图表组件 + 自由布局 + 实时数据刷新。系统预置常用图表(KPI 数字、折线、柱状、饼图、地图、表格),支持拖拽布局,数据自动 30 秒刷新。

### 2.3 关键算法或模型

- **数据聚合**: 后台按商户预聚合指标(销售/订单/客户/营销)
- **实时推送**: WebSocket 推送实时活动数据(下单、注册)
- **历史数据**: 从 OLAP/聚合表查询(过去 30 天/12 月趋势)
- **分享令牌**: 公开链接使用短期 token(7 天过期)

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 大屏名称 |
| 输入 | layout | object | 是 | 布局配置 |
| 输入 | widgets | array | 是 | 组件列表 |
| 输入 | refresh_interval | int | 否 | 刷新间隔(秒) |
| 输入 | is_public | bool | 否 | 是否公开 |
| 输出 | dashboard_id | int64 | 是 | 大屏 ID |
| 输出 | share_token | string | 否 | 分享令牌 |
| 输出 | snapshot | object | 是 | 当前数据快照 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/dashboards | 大屏列表 |
| POST | /api/dashboards | 创建大屏 |
| GET | /api/dashboards/:id | 大屏详情 |
| PUT | /api/dashboards/:id | 更新大屏 |
| DELETE | /api/dashboards/:id | 删除大屏 |
| GET | /api/dashboards/:id/data | 大屏数据 |
| POST | /api/dashboards/:id/share | 生成分享链接 |
| GET | /api/public/dashboards/:token | 公开大屏(无需登录) |
| GET | /api/dashboards/:id/realtime | 实时活动 |

### 3.3 安全与合规

- 公开大屏需 token,支持过期时间设置
- token 可随时撤销
- 大屏数据仅包含聚合指标,不含个人敏感信息
- 大屏只读,公开访问不暴露编辑入口

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 大屏加载 | < 1s |
| 数据刷新 | < 500ms (P95) |
| 实时推送延迟 | < 1s |
| 并发访问 | ≥ 200 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/dashboard | |
| Service | internal/service/dashboard | 大屏 CRUD + 数据组装 |
| Engine | internal/service/dashboard/aggregator | 指标聚合 |
| Repository | internal/repository/dashboard | |
| Model | internal/model/dashboard | |
| WS | internal/service/ws | 实时推送 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 订单/客户/事件 | 指标数据源 |
| WebSocket | 实时活动推送 |
| 自定义报表 | 部分组件可复用 |

### 4.3 数据流向

```text
[定时聚合任务] → [指标表]
                         ↓
[大屏请求] → [加载布局] → [并行查询各组件数据] → [实时数据 WS 推送] → [前端渲染]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"营销自动化 → 数据大屏"
2. 选择模板或空白创建
3. 拖拽组件(KPI/图表)到画布
4. 配置每个组件的数据源
5. 调整布局、颜色、字号
6. 预览效果
7. 保存并发布
8. 选择"公开分享"→ 生成短链
9. 复制链接,在展厅大屏展示

### 5.2 系统处理流程

1. 客户端打开大屏
2. 加载布局配置
3. 并行请求各组件数据(批量接口)
4. 渲染图表
5. 建立 WebSocket 连接,接收实时活动
6. 30 秒自动刷新数据
7. 公开访问通过 token 鉴权,只读

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 组件数据加载失败 | - | 组件级降级,显示"加载失败" |
| token 无效 | 403060 | 拒绝访问 |
| token 过期 | 403061 | 提示重新获取 |
| 实时连接断开 | - | 自动重连,降级为轮询 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `dashboards` | 大屏定义 |
| `dashboard_widgets` | 大屏组件 |
| `dashboard_share_tokens` | 分享 token |
| `metric_snapshots` | 指标快照(用于趋势图) |

```sql
CREATE TABLE dashboards (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(128) NOT NULL,
  description TEXT,
  layout JSONB NOT NULL,  -- 网格布局
  background VARCHAR(255),  -- 背景图
  theme VARCHAR(32) DEFAULT 'dark',  -- dark/light/tech
  refresh_interval INT DEFAULT 30,
  is_public BOOLEAN DEFAULT false,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_merchant ( deleted_at)
);

CREATE TABLE dashboard_widgets (
  id BIGINT PRIMARY KEY,
  dashboard_id BIGINT NOT NULL,
  widget_type VARCHAR(32) NOT NULL,  -- kpi/line/bar/pie/map/table/ranking
  title VARCHAR(128),
  position JSONB NOT NULL,  -- {x, y, w, h}
  data_source VARCHAR(64),  -- 数据源
  data_config JSONB,  -- 数据配置
  style_config JSONB,  -- 样式配置
  sort INT DEFAULT 0,
  INDEX idx_dashboard (dashboard_id)
);

CREATE TABLE dashboard_share_tokens (
  id BIGINT PRIMARY KEY,
  dashboard_id BIGINT NOT NULL,
  token VARCHAR(64) NOT NULL UNIQUE,
  password VARCHAR(255),  -- 可选访问密码
  expires_at TIMESTAMP,
  revoked BOOLEAN DEFAULT false,
  created_at TIMESTAMP NOT NULL,
  INDEX idx_token (token),
  INDEX idx_dashboard (dashboard_id)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 拖拽布局 | 拖动组件 | 位置持久化 | 待执行 |
| TC-002 | KPI 数字组件 | 销售额指标 | 正确显示 | 待执行 |
| TC-003 | 折线趋势图 | 30 天趋势 | 折线渲染 | 待执行 |
| TC-004 | 饼图 | 渠道占比 | 饼图渲染 | 待执行 |
| TC-005 | 地图组件 | 全国数据 | 地图高亮 | 待执行 |
| TC-006 | 实时活动推送 | 新订单 | 1s 内显示 | 待执行 |
| TC-007 | 自动刷新 | 30s 间隔 | 数据更新 | 待执行 |
| TC-008 | 公开分享 | 生成 token | 可访问 | 待执行 |
| TC-009 | token 过期 | 过期 token | 403 拒绝 | 待执行 |
| TC-010 | 撤销 token | revoke=true | 立即失效 | 待执行 |
| TC-011 | 密码保护 | 错误密码 | 拒绝 | 待执行 |
| TC-012 | 大屏降级 | 组件失败 | 降级提示 | 待执行 |
| TC-013 | 200 并发 | 公开大屏 | < 1s 响应 | 待执行 |
| TC-014 | 暗色主题 | 切换主题 | 正确切换 | 待执行 |
| TC-015 | 模板套用 | 销售大屏模板 | 一键应用 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 公开大屏 CDN | DASHBOARD_CDN_DOMAIN | - | 静态资源加速 |
| 实时推送 | WS_PUSH_ENABLED | true | |
| 默认刷新 | DASHBOARD_DEFAULT_REFRESH | 30s | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 大屏加载慢 | P95 > 1s | 钉钉 |
| 实时推送堆积 | > 1 万 | 钉钉 |
| 公开大屏被刷 | 异常 IP | 自动封禁 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.9 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
