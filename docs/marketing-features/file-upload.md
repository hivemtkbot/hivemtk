# 文件上传 (File Upload)

> **所属模块**: system
> **功能 slug**: `file-upload`
> **文档定位**: 通用文件上传服务,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 文件上传 |
| 功能名称(英文) | File Upload |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 分片上传/断点续传
- [x] 秒传(基于 hash)
- [x] 多媒体处理(图片压缩/缩略图)
- [x] 病毒扫描
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] 视频转码

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

系统中所有文件上传场景(头像、附件、导入、素材、卡片等)需要一个统一、安全、高效的上传服务。

### 2.3 关键算法或模型

- **分片策略**: 每片 5MB,按 partNumber 标识
- **秒传**: 计算 MD5,服务端已存在则返回 URL
- **断点续传**: 客户端记录已上传 part,服务端记录已合并
- **图片处理**: 缩略图/压缩/格式转换
- **病毒扫描**: ClamAV 集成

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | file | binary | 是 | 文件或分片 |
| 输入 | biz_type | string | 是 | 业务类型 |
| 输入 | hash | string | 否 | 文件 MD5(秒传) |
| 输入 | part_number | int | 否 | 分片序号(分片上传) |
| 输入 | upload_id | string | 否 | 上传 ID(分片上传) |
| 输出 | file_id | int64 | 是 | 文件 ID |
| 输出 | url | string | 是 | 访问 URL |
| 输出 | upload_id | string | 否 | 分片上传 ID |
| 输出 | is_instant | bool | 否 | 是否秒传 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/upload | 单文件上传(≤ 100MB) |
| POST | /api/upload/init | 初始化分片 |
| POST | /api/upload/part | 上传分片 |
| POST | /api/upload/merge | 合并分片 |
| POST | /api/upload/abort | 取消上传 |
| GET | /api/upload/status | 查询上传状态 |

### 3.3 安全与合规

- 文件类型白名单(MIME 嗅探)
- 文件大小限制(根据 biz_type)
- 病毒扫描
- 文件名清洗(防 XSS/路径穿越)
- 防盗链(签名 URL)
- 频率限制(每用户每分钟 20 次)

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 单文件上传 | < 2s (10MB) |
| 分片上传 | 5MB/分片,< 1s |
| 秒传命中 | < 100ms |
| 并发 | ≥ 100 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/upload | |
| Service | internal/service/upload | 统一上传 |
| Engine | internal/service/upload/chunked | 分片管理 |
| Engine | internal/service/upload/scanner | 病毒扫描 |
| Engine | internal/service/upload/processor | 图片处理 |
| Infra | internal/infra/obs | 存储 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 素材管理 | 调用上传 |
| 邮件附件 | 调用上传 |
| 卡片图片 | 调用上传 |

### 4.3 数据流向

```text
[客户端] → [单文件/分片上传] → [MD5 校验 + 病毒扫描] → [图片处理] → [OBS] → [返回 URL]
```

---

## 五、流程说明

### 5.1 用户操作流程(单文件)

1. 客户端选择文件
2. 计算 MD5
3. POST /upload 携带 MD5
4. 服务端检测已存在(秒传)或上传
5. 返回 URL

### 5.2 用户操作流程(分片)

1. 计算文件 MD5 + 切片
2. POST /upload/init → 获取 upload_id
3. 逐片 POST /upload/part
4. 全部完成 POST /upload/merge
5. 服务端合并 + 病毒扫描
6. 返回 URL

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 文件超大 | 413020 | 413 |
| 类型不支持 | 415020 | 415 |
| 病毒检出 | 400150 | 拒绝 |
| MD5 不匹配 | 400151 | 客户端重传 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `uploaded_files` | 已上传文件 |
| `upload_sessions` | 分片上传会话 |

```sql
CREATE TABLE uploaded_files (
  id BIGINT PRIMARY KEY,
  
  biz_type VARCHAR(32) NOT NULL,
  file_name VARCHAR(255) NOT NULL,
  file_size BIGINT NOT NULL,
  file_type VARCHAR(64),
  md5 VARCHAR(64) NOT NULL,
  oss_key VARCHAR(255) NOT NULL,
  url VARCHAR(512) NOT NULL,
  width INT,
  height INT,
  duration INT,
  is_instant BOOLEAN DEFAULT false,  -- 秒传
  scan_status VARCHAR(16) DEFAULT 'pending',  -- pending/clean/infected
  uploaded_by BIGINT,
  created_at TIMESTAMP NOT NULL,
  UNIQUE KEY uk_md5_merchant ( md5, biz_type),
  INDEX idx_biz (biz_type)
);

CREATE TABLE upload_sessions (
  id BIGINT PRIMARY KEY,
  upload_id VARCHAR(64) NOT NULL UNIQUE,
  
  file_name VARCHAR(255) NOT NULL,
  file_size BIGINT NOT NULL,
  md5 VARCHAR(64) NOT NULL,
  total_parts INT NOT NULL,
  uploaded_parts JSONB,  -- 已上传分片
  status VARCHAR(16) DEFAULT 'uploading',  -- uploading/merging/done/aborted
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP,
  INDEX idx_upload_id (upload_id),
  INDEX idx_expires (expires_at)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 小文件上传 | 1MB | URL | 待执行 |
| TC-002 | 中文件上传 | 50MB | URL | 待执行 |
| TC-003 | 大文件分片 | 500MB | 合并成功 | 待执行 |
| TC-004 | 秒传 | 重复文件 | 立即返回 | 待执行 |
| TC-005 | 断点续传 | 中断后继续 | 续传 | 待执行 |
| TC-006 | 类型不支持 | .exe | 415 | 待执行 |
| TC-007 | 病毒文件 | EICAR | 拒绝 | 待执行 |
| TC-008 | 文件名清洗 | `../etc/passwd` | 安全 | 待执行 |
| TC-009 | MD5 校验 | 篡改 | 拒绝 | 待执行 |
| TC-010 | 签名 URL | 防盗链 | 验证 | 待执行 |
| TC-011 | 图片压缩 | 5MB | < 1MB | 待执行 |
| TC-012 | 缩略图 | 上传 | 多尺寸 | 待执行 |
| TC-013 | 频率限制 | 21 次/分 | 拒绝 | 待执行 |
| TC-014 | 跨实例隔离 | 商户 A 文件 | 商户 B 不可见 | 待执行 |
| TC-015 | 取消上传 | 中途取消 | 清理 | 待执行 |
| TC-016 | 会话过期 | 24h 后 | 拒绝合并 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 分片大小 | UPLOAD_CHUNK_SIZE | 5MB | |
| 单文件最大 | UPLOAD_MAX_SIZE | 500MB | |
| 会话过期 | UPLOAD_SESSION_TTL | 24h | |
| ClamAV 地址 | CLAMAV_HOST | - | 病毒扫描 |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.11 节
- IMAGE_COMPRESSION.md
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
