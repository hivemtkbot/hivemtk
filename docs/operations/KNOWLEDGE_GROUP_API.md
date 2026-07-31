# 智能体知识库隔离 - API 参考

> API 风格: RESTful + JSON  
> 基础路径: `/api/v1/knowledge-bases`  
> 鉴权: Bearer Token (JWT)  
> 文档版本: 1.0 (2026-07-31)

---

## 1. 通用约定

### 1.1 通用响应格式

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... }
}
```

错误响应:
```json
{
  "code": 40001,
  "message": "name 不能为空",
  "data": null
}
```

### 1.2 错误码

| 错误码 | 含义 | HTTP |
|--------|------|------|
| 0 | 成功 | 200 |
| 40000 | 参数错误 | 400 |
| 40001 | 业务校验失败 | 400 |
| 40002 | 资源不存在 | 404 |
| 40003 | 越权访问 | 403 |
| 50000 | 服务器内部错误 | 500 |

### 1.3 通用字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 知识库 ID |
| kb_code | string | 业务唯一码 (KB-FAQ-001) |
| type | enum | faq / rag / sop |
| name | string | 知识库名称 |
| description | string | 描述 |
| owner_type | enum | private / shared |
| owner_agent_id | uint \| null | 当 owner_type=private 时必填 |
| enabled | bool | 是否启用 |
| member_count | int | 成员数 (冗余) |
| doc_count | int | 文档数 (冗余) |
| created_at | string (RFC3339) | 创建时间 |
| updated_at | string (RFC3339) | 更新时间 |

---

## 2. 知识库 CRUD

### 2.1 创建知识库

```http
POST /api/v1/knowledge-bases
Content-Type: application/json
Authorization: Bearer <token>

{
  "kb_code": "KB-FAQ-001",
  "type": "faq",
  "name": "客服常见问题",
  "description": "电商平台 FAQ",
  "owner_type": "private",
  "owner_agent_id": 1001,
  "enabled": true
}
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 123,
    "kb_code": "KB-FAQ-001",
    "type": "faq",
    "name": "客服常见问题",
    "owner_type": "private",
    "owner_agent_id": 1001,
    "enabled": true,
    "created_at": "2026-07-31T16:00:00Z",
    "updated_at": "2026-07-31T16:00:00Z"
  }
}
```

**业务校验失败 (400)**:
```json
{
  "code": 40001,
  "message": "owner_type=shared 时 owner_agent_id 必为空",
  "data": null
}
```

---

### 2.2 查询知识库

```http
GET /api/v1/knowledge-bases/{id}
Authorization: Bearer <token>
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 123,
    "kb_code": "KB-FAQ-001",
    "type": "faq",
    ...
  }
}
```

**资源不存在 (404)**:
```json
{
  "code": 40002,
  "message": "knowledge base not found",
  "data": null
}
```

---

### 2.3 更新知识库

```http
PATCH /api/v1/knowledge-bases/{id}
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "更新后的名称",
  "description": "新描述",
  "enabled": false
}
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 123,
    "name": "更新后的名称",
    ...
  }
}
```

---

### 2.4 删除知识库

```http
DELETE /api/v1/knowledge-bases/{id}
Authorization: Bearer <token>
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

**业务说明**: 删除 KB 时, 同步级联删除所有 `agent_kb_bindings` 引用。

---

## 3. 知识库查询

### 3.1 列表查询 (管理端)

```http
GET /api/v1/knowledge-bases?type=faq&owner_type=private&agent_id=1001&keyword=客服&page=1&page_size=20
Authorization: Bearer <token>
```

**Query 参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| type | string | 否 | faq / rag / sop |
| owner_type | string | 否 | private / shared |
| agent_id | uint | 否 | 按智能体过滤 |
| enabled | bool | 否 | 按启用状态 |
| keyword | string | 否 | 按名称/描述模糊 |
| page | int | 否 | 页码, 默认 1 |
| page_size | int | 否 | 每页条数, 默认 20, 最大 200 |

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "total": 42,
    "page": 1,
    "page_size": 20,
    "items": [
      {
        "id": 123,
        "kb_code": "KB-FAQ-001",
        ...
      }
    ]
  }
}
```

---

### 3.2 智能体可见知识库 (业务端)

```http
GET /api/v1/agents/{agent_id}/knowledge-bases
Authorization: Bearer <token>
```

**业务逻辑**: 返回 `agent_id` 可见的 KB = 私有 ∪ 显式绑定的共享

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 123,
      "kb_code": "KB-FAQ-001",
      "type": "faq",
      "owner_type": "private",
      "owner_agent_id": 1001,
      ...
    },
    {
      "id": 456,
      "kb_code": "KB-SOP-PLATFORM",
      "type": "sop",
      "owner_type": "shared",
      "owner_agent_id": null,
      ...
    }
  ]
}
```

---

### 3.3 共享知识库列表

```http
GET /api/v1/knowledge-bases/shared?type=sop
Authorization: Bearer <token>
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 456,
      "kb_code": "KB-SOP-PLATFORM",
      "type": "sop",
      "owner_type": "shared",
      ...
    }
  ]
}
```

---

## 4. 智能体绑定

### 4.1 单个绑定

```http
POST /api/v1/agents/{agent_id}/knowledge-bases/{kb_id}
Content-Type: application/json
Authorization: Bearer <token>

{
  "priority": 10
}
```

**业务说明**:
- 重复绑定 (agent, kb) 自动覆盖
- kb 必须存在

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": 789,
    "agent_id": 1001,
    "kb_id": 456,
    "role": "primary",
    "priority": 10,
    "enabled": true,
    "created_at": "2026-07-31T16:00:00Z"
  }
}
```

---

### 4.2 单个解绑

```http
DELETE /api/v1/agents/{agent_id}/knowledge-bases/{kb_id}
Authorization: Bearer <token>
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": null
}
```

---

### 4.3 智能体的所有绑定

```http
GET /api/v1/agents/{agent_id}/knowledge-bases/bindings
Authorization: Bearer <token>
```

**Query 参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| kb_type | string | 否 | faq / rag / sop |

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 789,
      "agent_id": 1001,
      "kb_id": 456,
      "kb_type": "sop",
      "role": "primary",
      "priority": 10,
      "enabled": true
    }
  ]
}
```

---

### 4.4 知识库的引用智能体

```http
GET /api/v1/knowledge-bases/{kb_id}/agents
Authorization: Bearer <token>
```

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": [
    {
      "id": 789,
      "agent_id": 1001,
      "kb_id": 456,
      "role": "primary",
      "priority": 10,
      "enabled": true
    }
  ]
}
```

---

### 4.5 批量绑定

```http
POST /api/v1/agent-kb-bindings/batch
Content-Type: application/json
Authorization: Bearer <token>

{
  "items": [
    {"agent_id": 1001, "kb_id": 456, "priority": 1},
    {"agent_id": 1001, "kb_id": 789, "priority": 2},
    {"agent_id": 1002, "kb_id": 456, "priority": 1}
  ]
}
```

**业务说明**:
- 事务性: 任一失败, 全部回滚
- 重复 binding 自动覆盖

**响应 (200)**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "success_count": 3,
    "failed_count": 0
  }
}
```

**失败 (400)**:
```json
{
  "code": 40001,
  "message": "items[agent=1001 kb=99999]: 知识库不存在",
  "data": {
    "success_count": 0,
    "failed_count": 3
  }
}
```

---

## 5. 业务规则示例

### 5.1 创建共享 KB (跨智能体可见)

```bash
# 1. 创建 shared KB
curl -X POST http://localhost:8080/api/v1/knowledge-bases \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <admin_token>" \
  -d '{
    "kb_code": "KB-SOP-PLATFORM",
    "type": "sop",
    "name": "平台通用 SOP",
    "owner_type": "shared"
  }'

# 2. 给 agent1 绑定
curl -X POST http://localhost:8080/api/v1/agents/1001/knowledge-bases/456 \
  -H "Authorization: Bearer <admin_token>"

# 3. agent1 可见此 shared KB
curl http://localhost:8080/api/v1/agents/1001/knowledge-bases \
  -H "Authorization: Bearer <agent1_token>"
```

### 5.2 升级 private → shared

```bash
# 1. 起初是 private
curl -X POST http://localhost:8080/api/v1/knowledge-bases \
  -d '{
    "kb_code": "KB-FAQ-TEAM",
    "type": "faq",
    "name": "团队 FAQ",
    "owner_type": "private",
    "owner_agent_id": 1001
  }'

# 2. 升级为 shared (清空 owner)
curl -X PATCH http://localhost:8080/api/v1/knowledge-bases/123 \
  -H "Content-Type: application/json" \
  -d '{
    "owner_type": "shared",
    "owner_agent_id": null
  }'

# 3. 现在需要 binding 才能访问
curl -X POST http://localhost:8080/api/v1/agents/1002/knowledge-bases/123
```

### 5.3 删除 KB (级联清理)

```bash
# 1. 删除 KB
curl -X DELETE http://localhost:8080/api/v1/knowledge-bases/123 \
  -H "Authorization: Bearer <admin_token>"

# 2. 自动级联: 所有 (agent_id, 123) binding 消失
# 3. 验证
curl http://localhost:8080/api/v1/knowledge-bases/123/bindings
# 期望: {"code": 40002, "message": "knowledge base not found"}
```

---

## 6. 错误处理最佳实践

### 6.1 客户端处理

```javascript
async function createKB(data) {
  const resp = await fetch('/api/v1/knowledge-bases', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  const json = await resp.json();
  if (json.code !== 0) {
    // 业务错误
    if (json.code === 40001) {
      throw new ValidationError(json.message);
    }
    // 系统错误
    throw new ApiError(json.message);
  }
  return json.data;
}
```

### 6.2 重试策略

| 错误码 | 重试策略 |
|--------|----------|
| 0 | 成功, 无需重试 |
| 40000 / 40001 | 不重试 (业务校验) |
| 40002 | 不重试 (资源不存在) |
| 40003 | 不重试 (越权) |
| 50000 | 重试 3 次, 指数退避 |

---

## 7. 性能与限制

### 7.1 速率限制

| 端点 | 每分钟限制 |
|------|-----------|
| GET (查询) | 600 req/min |
| POST (创建) | 60 req/min |
| PATCH (更新) | 120 req/min |
| DELETE (删除) | 60 req/min |
| 批量绑定 | 10 req/min |

### 7.2 数据规模

| 项 | 上限 |
|----|------|
| 单智能体 KB 数 | 1000 |
| 单 KB binding 数 | 5000 |
| 批量绑定单次 | 200 |

---

## 8. 客户端 SDK 示例

### 8.1 Go SDK

```go
import "marketing/internal/client"

client := client.New("http://user-server:8080", "bearer-token")

// 创建
kb, err := client.KnowledgeBase.Create(ctx, &client.KBCreateRequest{
    KBCode:    "KB-FAQ-001",
    Type:      "faq",
    Name:      "客服 FAQ",
    OwnerType: "private",
    OwnerAgentID: 1001,
})

// 智能体可见 KB
kbs, err := client.Agent.ListVisibleKBs(ctx, 1001)
```

### 8.2 JavaScript SDK

```javascript
import { HiveMTKClient } from '@hivemtk/sdk';

const client = new HiveMTKClient({
  baseURL: 'http://user-server:8080',
  token: 'bearer-token'
});

const kbs = await client.agent.listVisibleKBs(1001);
```

---

## 9. OpenAPI 规范

完整 OpenAPI 3.0 规范: `api/openapi/knowledge_group.yaml`

主要端点摘要:

```yaml
openapi: 3.0.0
info:
  title: HiveMTK Knowledge Group API
  version: 1.0.0
paths:
  /api/v1/knowledge-bases:
    post:
      summary: 创建知识库
      tags: [KnowledgeBase]
  /api/v1/knowledge-bases/{id}:
    get: {summary: 查询知识库}
    patch: {summary: 更新知识库}
    delete: {summary: 删除知识库 (级联清理 binding)}
  /api/v1/agents/{agent_id}/knowledge-bases:
    get: {summary: 智能体可见 KB 列表}
  /api/v1/agents/{agent_id}/knowledge-bases/{kb_id}:
    post: {summary: 绑定 KB}
    delete: {summary: 解绑 KB}
  /api/v1/agent-kb-bindings/batch:
    post: {summary: 批量绑定 (事务)}
```

---

## 10. 相关文档

- `docs/architecture/KNOWLEDGE_GROUP_DESIGN.md` - 架构设计
- `docs/architecture/adr/ADR-014-knowledge-group-isolation.md` - ADR
- `docs/operations/KNOWLEDGE_GROUP_DEPLOY.md` - 部署
- `docs/operations/KNOWLEDGE_GROUP_MONITORING.md` - 监控

---

**最后更新**: 2026-07-31  
**作者**: HiveMTK API 团队
