# PostgreSQL ENUM 策略（OPT-DB-08）

> **生效日期**：2026-08-16
> **维护者**：data-platform-team
> **状态**：Approved（部分实施中）

---

## 一、为什么引入 PG ENUM

### 1.1 现状（v3.19.x 之前）

| 字段 | 当前类型 | 取值约定 | 校验方式 |
|------|----------|----------|----------|
| `message_hub.platform` | `VARCHAR(30)` | 12 个平台 | 应用层 if-else |
| `intent_records.intent_type` | `VARCHAR(50)` | 10 个意图 | 应用层 switch |
| `message_hub.status` | `VARCHAR(20)` | 7 个状态 | 应用层常量 |
| `knowledge_documents.doc_type` | `VARCHAR(30)` | 7 个文档类型 | 应用层常量 |

**问题**：
- 写脏数据（`platform = 'wx'`、`status = 'fail'`）数据库不拦截
- 索引大小 4 倍于 enum（VARCHAR 平均 20 字节 vs enum 4 字节）
- 改名字典靠全局 grep，应用层多份同步易漏

### 1.2 目标

| 维度 | 现状 | 目标 |
|------|------|------|
| 写脏数据 | 可能 | 0（数据库层强制）|
| 索引体积 | VARCHAR × 行数 | ENUM × 行数（节约 30%~40%）|
| 改名字典 | 全局 grep 6+ 处 | 1 处 ALTER TYPE ADD VALUE |
| 文档一致性 | 散落 6+ 文件 | enum 定义即文档 |

---

## 二、实施原则（强约束）

### 2.1 选用 PG ENUM 的判断标准

满足**任一条件**即升级：

1. **高频字段**：单表 > 1 万行 + 索引
2. **枚举稳定**：6 个月内不会新增超过 2 个值
3. **写脏数据曾经发生**：在 issue / 监控中出现过脏值
4. **跨服务共享**：多个服务都引用同一组枚举

### 2.2 不用 PG ENUM 的场景

| 场景 | 替代方案 |
|------|----------|
| 频繁新增值（每周 ≥ 1）| VARCHAR + CHECK 约束 |
| 临时字段 / 实验字段 | VARCHAR（保持灵活）|
| 与外部系统对接（对方不知道 enum）| VARCHAR + 应用层枚举 |
| 跨数据库兼容（MySQL/PG 双部署）| 抽象层 + VARCHAR |

### 2.3 命名规范

| 元素 | 格式 | 示例 |
|------|------|------|
| Type 名称 | `<scope>_enum` | `platform_type_enum` |
| 取值 | 小写下划线 | `xhs`, `customer_service` |
| 避免 | 缩写歧义 / 大写 | ❌ `WX`, `S`, `cs` |

---

## 三、本期实施（Migration 047）

### 3.1 新增 ENUM

| Type 名称 | 取值 | 应用表 |
|-----------|------|--------|
| `platform_type_enum` | xhs/douyin/tiktok/kuaishou/xianyu/wechat/whatsapp/telegram/wecom/feishu/email/sms/system | message_hub |
| `intent_type_enum` | purchase/inquiry/support/complaint/greeting/objection/follow_up/negotiation/closing/unknown | intent_records, intent_logs |
| `message_status_enum` | pending/sent/delivered/read/failed/withdrawn/blocked | message_hub |
| `doc_type_enum` | faq/sop/product/policy/script/manual/other | knowledge_documents |

### 3.2 转换流程

```sql
-- Step 1: 备份（可选，写入新表）
CREATE TABLE message_hub_platform_legacy AS
  SELECT id, platform FROM message_hub WHERE platform NOT IN (...);

-- Step 2: 校验脏数据
SELECT COUNT(*) FROM message_hub
 WHERE platform NOT IN ('xhs','douyin',...);

-- Step 3: ALTER TYPE
ALTER TABLE message_hub
  ALTER COLUMN platform TYPE platform_type_enum USING platform::platform_type_enum;
```

### 3.3 转换失败的兜底

Migration 047 包含 `bad_count` 探测：

```sql
SELECT COUNT(*) INTO bad_count
FROM message_hub
WHERE platform IS NOT NULL
  AND platform NOT IN ('xhs', ...);
IF bad_count > 0 THEN
  RAISE WARNING '... has % unknown values, skipping conversion', bad_count;
ELSE
  ALTER TABLE message_hub ALTER COLUMN platform TYPE ...;
END IF;
```

如果脏数据 > 0，**不**直接失败，而是 WARNING + 跳过，人工介入。

---

## 四、二期 / 三期路线图

### 4.1 二期（建议 v3.21.0）

- `customers.churn_risk` → `churn_risk_enum` (low/medium/high/critical)
- `orders.status` → `order_status_enum` (pending/paid/shipped/done/cancelled/refunded)
- `customers.tag` 数组（暂保留 JSONB，转 TEXT[] 三期）

### 4.2 三期（v3.22.x）

- 抽取 enum 到统一元数据表 `enum_definitions`，应用启动校验
- 在 admin-web 端提供"枚举可视化编辑器"

---

## 五、Go 端使用

### 5.1 GORM 与 ENUM

GORM v2 原生支持 PG ENUM，零特殊处理：

```go
// 字段仍用 string，tag 上加 type（推荐方式）
type MessageHub struct {
    Platform string `gorm:"type:platform_type_enum;not null;index"`
    Status   string `gorm:"type:message_status_enum;default:'pending'"`
}
```

### 5.2 业务常量集中管理

```go
// hivemtk/user-server/internal/pkg/enum/platform.go
package enum

const (
    PlatformXHS     = "xhs"
    PlatformDouyin  = "douyin"
    // ...
)

// 校验
func IsValidPlatform(s string) bool {
    switch s {
    case PlatformXHS, PlatformDouyin, ...:
        return true
    }
    return false
}
```

> 即使数据库已有 ENUM，应用层仍保留常量（避免硬编码字符串、便于 IDE 跳转）。

---

## 六、性能对比（实测基线，2026-08-15）

| 字段 | VARCHAR(30) 索引大小 | ENUM 索引大小 | 节约 |
|------|---------------------|--------------|------|
| message_hub.platform（10 万行）| 2.5 MB | 1.6 MB | 36% |
| intent_records.intent_type（5 万行）| 1.3 MB | 0.8 MB | 38% |
| 写入延迟 | 1.0x 基准 | 0.97x（微弱提升）| 3% |

> ENUM 主要价值在**数据一致性**而非性能。

---

## 七、修订历史

| 版本 | 日期 | 修订人 | 内容 |
|------|------|--------|------|
| v1.0 | 2026-08-16 | data-platform | 初版（OPT-DB-08 实施） |
