# OBS 配置 (OBS Config)

> **所属模块**: system
> **功能 slug**: `obs-config`
> **文档定位**: 对象存储服务配置管理,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | OBS 配置 |
| 功能名称(英文) | Object Storage Config |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P1 |
| 负责人 | |
| 计划完成时间 | |
| 实际完成时间 | 2026-07-14 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端配置管理页
- [x] 多 OBS 厂商支持(华为云/阿里云/腾讯云/七牛云)
- [x] 测试连接 + 默认设置
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] AWS S3 兼容

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户素材、邮件附件、卡片图片、备份文件等均需对象存储。不同商户可能使用不同云厂商,需支持多家 OBS 配置与切换。

### 2.2 解决思路

抽象统一 OBS 接口,适配各厂商 SDK(华为云 OBS、阿里云 OSS、腾讯云 COS、七牛云),配置可保存多份,设置默认。

### 2.3 关键算法或模型

- **抽象层**: `internal/infra/obs/client.go` 定义统一接口
- **适配器**: huawei/alibaba/tencent/七牛云 各实现
- **配置加密**: AK/SK AES-256-GCM 加密
- **测试连接**: 上传测试小文件并删除

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 配置名称 |
| 输入 | provider | string | 是 | huawei/alibaba/tencent/七牛云 |
| 输入 | endpoint | string | 是 | 端点 |
| 输入 | bucket | string | 是 | 桶名 |
| 输入 | access_key | string | 是 | AK |
| 输入 | secret_key | string | 是 | SK |
| 输入 | region | string | 否 | 区域 |
| 输入 | is_default | bool | 否 | 是否默认 |
| 输出 | config_id | int64 | 是 | 配置 ID |
| 输出 | test_result | object | 是 | 测试结果 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/obs-config | 配置列表 |
| POST | /api/obs-config | 创建 |
| GET | /api/obs-config/:id | 详情(SK 脱敏) |
| PUT | /api/obs-config/:id | 更新 |
| DELETE | /api/obs-config/:id | 删除 |
| POST | /api/obs-config/:id/test | 测试连接 |
| POST | /api/obs-config/:id/default | 设为默认 |

### 3.3 安全与合规

- AK/SK 加密存储
- 仅管理员可操作
- 测试连接不影响业务
- 审计日志

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 测试连接 | < 3s |
| 配置查询 | < 100ms |
| 文件上传(经 OBS) | < 2s (1MB) |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/obs_config | |
| Service | internal/service/obs_config | |
| Repository | internal/repository/obs_config | |
| Model | internal/model/obs_config | |
| Infra | internal/infra/obs | 各厂商客户端 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 素材管理 | 使用 OBS |
| 邮件附件 | 使用 OBS |
| 备份恢复 | 使用 OBS |
| 卡片图片 | 使用 OBS |

### 4.3 数据流向

```text
[配置变更] → [加密入库] → [清除缓存] → [业务模块使用新配置]
[上传文件] → [通过默认配置上传] → [OBS 桶]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"系统管理 → OBS 配置"
2. 点击"新增配置"
3. 选择厂商 + 填写参数
4. 点击"测试连接"验证
5. 保存
6. 可选"设为默认"

### 5.2 系统处理流程

1. 接收配置
2. AK/SK 加密
3. 入库
4. 测试连接:创建桶测试对象 → 删除
5. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 端点不可达 | 500140 | 提示 |
| 认证失败 | 500141 | 提示 |
| 桶不存在 | 500142 | 提示 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `obs_configs` | OBS 配置 |

```sql
CREATE TABLE obs_configs (
  id BIGINT PRIMARY KEY,
  
  name VARCHAR(64) NOT NULL,
  provider VARCHAR(32) NOT NULL,  -- huawei/alibaba/tencent/七牛云
  endpoint VARCHAR(255) NOT NULL,
  bucket VARCHAR(128) NOT NULL,
  access_key VARCHAR(255) NOT NULL,  -- 加密
  secret_key VARCHAR(255) NOT NULL,  -- 加密
  region VARCHAR(64),
  is_default BOOLEAN DEFAULT false,
  is_https BOOLEAN DEFAULT true,
  cdn_domain VARCHAR(255),
  status VARCHAR(16) DEFAULT 'active',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  deleted_at TIMESTAMP,
  INDEX idx_merchant ( deleted_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建配置 | 完整参数 | 200 OK | 待执行 |
| TC-002 | 加密存储 | AK/SK | DB 中加密 | 待执行 |
| TC-003 | 测试连接 | 正确参数 | 成功 | 待执行 |
| TC-004 | 测试失败 | 错误 SK | 失败提示 | 待执行 |
| TC-005 | 端点不可达 | 错端点 | 失败提示 | 待执行 |
| TC-006 | 桶不存在 | 不存在桶 | 失败提示 | 待执行 |
| TC-007 | 设为默认 | 多配置切换 | 单一默认 | 待执行 |
| TC-008 | 删除默认 | 默认配置 | 拒绝或自动切换 | 待执行 |
| TC-009 | 跨厂商切换 | 华为→阿里 | 业务使用新 | 待执行 |
| TC-010 | 脱敏返回 | SK 字段 | 不可见 | 待执行 |
| TC-011 | 七牛云 自建 | 内网端点 | 正常 | 待执行 |
| TC-012 | HTTPS 切换 | 协议 | 正确 | 待执行 |
| TC-013 | CDN 域名 | 填写 | 拼接使用 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 加密密钥 | OBS_AES_KEY | - | |
| 默认 provider | OBS_DEFAULT_PROVIDER | huawei | |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| OBS 调用失败 | > 5% | 钉钉 |
| 测试连接失败 | 持续 | 邮件 |

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- IMAGE_COMPRESSION.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档生成 | |
