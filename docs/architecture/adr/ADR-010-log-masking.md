# ADR-010: 日志与敏感信息脱敏

- **状态**：已合并到 `docs/standards/MASTER_RULES.md` 第 4.5 节
- **范围**：所有后端服务 + 前端
- **原始编号**：DOC-LOG-001

## 背景

日志不规范导致的安全事件：

- 密码 / Token 写入日志，被搜索引擎抓取
- 用户手机号 / 邮箱明文出现在日志（违反敏感数据保护规范）
- 信用卡号 / 身份证号明文（违反 PCI-DSS）
- 调试日志泄露 SQL（包含表结构 / 业务数据）

## 决策

**已合并到 `MASTER_RULES.md` §4.5**，核心规范：

### 1. 敏感字段字典

| 字段名 | 类型 | 脱敏方式 | 示例 |
|--------|------|----------|------|
| password | string | 完整替换 | `***` |
| token / jwt | string | 保留前 4 + 后 4 | `eyJh****...xxxx` |
| phone / mobile | string | 中间 4 位 `*` | `138****1234` |
| email | string | 用户名前 2 字符 + `***@domain` | `ab***@example.com` |
| id_card | string | 前 6 + 后 4 中间全 `*` | `110101****1234` |
| bank_card | string | 仅保留后 4 | `****1234` |
| address | string | 保留省市，详细地址 `*` | `北京市朝阳区****` |

### 2. 实现机制

```go
// 自动检测 key 名（不依赖 value 内容）
func Sanitize(key string, value any) any {
    if isSensitiveKey(key) {
        return maskValue(key, value)
    }
    return value
}
```

- 日志库 hook 注入（zerolog / zap 的 `Hook`）
- SQL 日志：使用 `gorm.io/plugin/dbresolver` 配合 `RequestID` 上下文
- 异常堆栈：使用 `errors.WithStack` 截断敏感帧

### 3. 强制检查项

- CI 中 `gitleaks` 扫描（防止密钥硬编码）
- 测试中 `assertNoSensitive(logs)` 断言
- 数据库 audit 表记录"谁在何时读了敏感数据"

### 4. 日志分级

| 级别 | 用途 | 是否落盘 |
|------|------|----------|
| DEBUG | 调试 | 仅 dev |
| INFO | 流程 | 7 天 |
| WARN | 异常但可恢复 | 30 天 |
| ERROR | 业务错误 | 90 天 |
| FATAL | 系统崩溃 | 永久 |

### 5. 链路追踪

- 每次请求生成 `request_id`（UUIDv7）
- 跨服务透传（HTTP `X-Request-ID` header）
- 日志自动附加 `request_id`、`user_id`

## 后果

### 正面

- 通过日志安全自检（私域部署无外部合规审计）
- 敏感字段脱敏落地
- 日志泄露事件降为 0

### 负面

- 日志中可读性下降（排查问题需要看原始值时需开启 DEBUG）
- 性能开销约 3%（脱敏逻辑）

## 落地

- `internal/pkg/utils/logger/sanitize.go`
- `internal/middleware/request_id.go`
- `tests/log_safety_test.go`

## 关联

- ADR-009：错误处理（错误日志脱敏）
- ADR-001：五层架构（脱敏在 service 层完成）
