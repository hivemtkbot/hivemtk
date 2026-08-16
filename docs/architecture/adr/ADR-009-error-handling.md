# ADR-009: 错误处理规范

- **状态**：已合并到 `docs/standards/MASTER_RULES.md` 第 4.3 节
- **范围**：所有 Go 后端服务
- **原始编号**：DOC-ERR-001

## 背景

项目早期错误处理五花八门：

- 业务错误用 `errors.New("xxx")`，调用方无法判断错误类型
- HTTP 状态码与错误码混用（400 / 401 / 403 / 500）
- 错误信息泄露内部细节（SQL、堆栈）
- 错误日志缺少 trace_id，无法串联链路

## 决策

**已合并到 `MASTER_RULES.md` §4.3**，核心规范：

### 1. 错误分类

```go
// 业务错误（可恢复）
type BizError struct {
    Code    int    // 业务错误码：5 位数字
    Message string // 用户可读
    Cause   error  // 原始错误（不暴露给前端）
}

// 系统错误（不可恢复）
type SysError struct {
    Code    int    // 系统错误码：6xxx
    Message string
    Cause   error
}
```

### 2. 错误码分段

| 段位 | 含义 | HTTP |
|------|------|------|
| 0 | 成功 | 200 |
| 1xxx | 通用客户端错误 | 400 |
| 2xxx | 鉴权错误 | 401 / 403 |
| 3xxx | 资源错误 | 404 / 409 |
| 4xxx | 第三方错误 | 502 / 503 |
| 5xxx | 业务规则错误 | 422 |
| 6xxx | 系统错误 | 500 |

### 3. Controller 层处理

```go
if err != nil {
    if bizErr, ok := err.(*BizError); ok {
        response.Error(ctx, httpStatus, bizErr.Code, bizErr.Message)
        return
    }
    // 未知错误
    response.Error(ctx, 500, 6000, "系统繁忙")
    logger.Error(ctx, "unhandled", "err", err)
    return
}
```

### 4. 错误响应格式

```json
{
  "code": 1001,
  "message": "参数错误：name 不能为空",
  "request_id": "abc-123-def"
}
```

### 5. 日志规范

- 所有错误必须记录到结构化日志（含 trace_id）
- DB 错误必须用 `response.ErrorFromDB(ctx, err, msg)`
- 不在响应中暴露内部错误对象

## 后果

### 正面

- 前端可通过 `code` 字段精确判断错误类型
- 错误码文档化（`docs/api/error-codes.md`）
- 全链路 trace_id 串联

### 负面

- 老代码重构工作量大
- 需要维护错误码字典

## 落地

- `internal/pkg/utils/response/error.go`
- `internal/middleware/error_handler.go`
- `controller/error_helper.go`

## 关联

- ADR-001：五层架构（错误处理分层）
- ADR-005：数据库设计（DB 错误码映射）
- ADR-008：触达限流（限流错误的特殊处理）
