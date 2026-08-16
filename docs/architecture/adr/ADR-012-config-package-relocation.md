# ADR-012：config 包整体迁移至 internal/config/

| 字段 | 内容 |
|------|------|
| 编号 | ADR-012 |
| 标题 | config 包整体迁移至 internal/config/ |
| 状态 | ✅ Accepted（已执行）|
| 决策者 | @maintainer-team |
| 日期 | 2026-08-10 |
| 适用范围 | user-server 配置包位置合规化 |
| **实施 PR** | #295（config 包迁移）|
| **已部署环境** | dev / staging / 客户 A（生产 v3.18.0+）|

## 一、背景

### 1.1 现状

user-server 当前存在**两个同名 `config` 包**，分散在不同层级：

| 路径 | 职责 | 层级定位 |
|---|---|---|
| `internal/config/` | AdminConfig / PlatformConfig / LoadPlatform | 横向：平台配置加载（合规） |
| `internal/pkg/utils/config/` | AppConfig / InferenceConfig / DatabaseConfig / GetAppConfig / GetRootDir / GetEnvDir | 误放在"通用工具"层（违规） |

### 1.2 问题

1. **层级违规**：项目分层规范（MASTER_RULES.md）明确规定
   - `internal/config/` → 横向：平台配置加载
   - `internal/pkg/utils/` → 通用工具（无业务含义），仅含 `db/ logger/ jwt/ bcrypt/ response/ pagination/ mail/ cron/ type/`
   - `AppConfig` 含 `InferenceConfig`（LLM/Embedding/Rerank 业务配置），**绝非"无业务含义的通用工具"**

2. **包分裂**：两个 `config` 包同名但 import 路径不同
   - `marketing/internal/config`（admin/platform）
   - `marketing/internal/pkg/utils/config`（app/inference/database）
   - 新人极易 import 错误，且难以发现

3. **CI 缺口**：check-architecture.sh 原本只检查 controller/service/repository/dto/model 分层，未覆盖 config 包位置（本 ADR 配套已新增 [10/10] 检查项）

### 1.3 触发事件

2026-07-26 对 `inference_load_test.go` 的全角度审查发现此问题，列为 P0 架构违规。详见头脑风暴论证报告。

---

## 二、决策

### 2.1 迁移目标

将 `internal/pkg/utils/config/` 全部文件迁移至 `internal/config/`，与现有 AdminConfig/PlatformConfig 合并为单一 config 包：

```
internal/config/
├── admin.go              # 已存在（AdminConfig）
├── admin_test.go         # 已存在
├── platform.go           # 已存在（PlatformConfig）
├── platform_test.go      # 已存在
├── app.go                # 新增（从 server.go 拆分：AppConfig + GetAppConfig）
├── inference.go          # 新增（从 server.go 拆分：InferenceConfig 系列）
├── database.go           # 新增（从 server.go 拆分：DatabaseConfig / PostgresConfig / PoolConfig）
├── storage.go            # 新增（从 server.go 拆分：StorageConfig / QiniuConfig）
├── i18n.go               # 新增（从 server.go 拆分：I18nConfig 系列）
├── vector.go             # 新增（从 server.go 拆分：VectorDatabaseConfig / PGVectorConfig）
├── init.go               # 迁移（GetRootDir / GetEnvDir）
├── app_test.go           # 迁移并重命名（原 config_test.go）
└── inference_load_test.go # 迁移
```

### 2.2 命名规范

- `server.go` 拆分为多个按业务域命名的文件（`app.go` / `inference.go` / `database.go` 等），符合架构文档 §2.2 "<domain>.go" 规则
- `server.go` 命名过于通用，不符合业务域命名原则，迁移时拆分

### 2.3 import 路径变更

全量替换：
```
旧：marketing/internal/pkg/utils/config
新：marketing/internal/config
```

预计影响文件数：约 30-50 个（所有 import config 的 .go 文件）

### 2.4 不迁移的内容

`internal/pkg/utils/config/init.go` 中的 `GetRootDir` / `GetEnvDir` 是路径工具函数，**严格属于"通用工具"**。但为避免包分裂，本次一并迁入 `internal/config/`。未来若需进一步解耦，可将这两个函数迁至 `internal/pkg/utils/fileutil/`。

---

## 三、迁移步骤

### 3.1 准备（不破坏现状）

1. 在 `internal/config/` 下新建按业务域命名的文件，复制对应内容
2. 确保 `internal/config/` 包能独立编译
3. 运行 `go build ./internal/config/...` 验证

### 3.2 切换（原子提交）

1. 全量替换 import 路径：`marketing/internal/pkg/utils/config` → `marketing/internal/config`
2. 删除 `internal/pkg/utils/config/` 目录
3. 运行 `go build ./...` 验证
4. 运行 `go test ./...` 验证
5. 运行 `bash scripts/check-architecture.sh` 验证 [10/10] 通过

### 3.3 清理

1. 删除空目录 `internal/pkg/utils/config/`
2. 更新项目规范文档（标注迁移完成）
3. 更新本 ADR 状态为"已执行"

---

## 四、风险与缓解

| 风险 | 缓解 |
|---|---|
| import 路径全量替换遗漏 | 用 `grep -r "marketing/internal/pkg/utils/config"` 兜底确认 |
| 两个 config 包合并后符号冲突 | 迁移前检查 AdminConfig/PlatformConfig 与 AppConfig 无重名符号（已确认无冲突） |
| 测试相对路径失效 | `inference_load_test.go` 的 `../../../config.yaml` 路径层级会变化（从 4 级变 3 级），需同步调整 |
| 外部脚本引用旧路径 | 检查 scripts/ 下是否有硬编码路径引用 |

---

## 五、验收标准

1. `go build ./...` 通过
2. `go test ./...` 通过
3. `bash scripts/check-architecture.sh` 通过，[10/10] 显示"config 包位置合规"
4. `grep -r "marketing/internal/pkg/utils/config"` 返回 0 结果
5. `internal/pkg/utils/config/` 目录不存在

---

## 六、时间线

- **立项**：2026-07-26（本 ADR）
- **执行**：2026-08-10（P0-P3 架构整改一次性落地）
- **完成**：2026-08-10

### 执行偏差记录

1. **未拆分 server.go**：原计划按业务域拆为 app.go/inference.go/database.go 等，
   实际以整文件 `server.go` 原样迁入 `internal/config/`（控制单次变更风险）；
   文件内类型已按域分段注释，拆分可作为后续纯重命名类小改动跟进。
2. **符号冲突处置**：`type PlatformCfg struct`（yaml 解析载体）与
   `internal/config` 已有的 `var PlatformCfg *PlatformConfig` 同名，
   前者重命名为非导出 `platformAPIYAML`（仅 GetServerBaseURL 内部使用，零外部影响）。
3. **测试路径**：inference_load_test.go 相对路径由 4 级上改为 2 级上
   （`internal/config` 相对 user-server 根目录仅 2 级）。

---

## 七、关联

- **前置 ADR**：无
- **关联 ADR**：ADR-013（模块名 marketing → user-server），建议同步执行
- **关联规范**：项目分层规范（参见 MASTER_RULES.md）
- **关联检查**：scripts/check-architecture.sh [10/10] Config 包位置检查
- **触发事件**：2026-07-26 inference_load_test.go 全角度审查头脑风暴

## 修订历史

| 版本 | 日期 | 修订人 | 内容 |
|------|------|--------|------|
| v1.0 | 2026-08-10 | @maintainer-team | 初版 config 包迁移决策 |
| v1.1 | 2026-08-16 | audit-agent | 增补"实施 PR"和"已部署环境"字段 |
