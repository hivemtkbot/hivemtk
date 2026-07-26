# user-web 与 user-server 工程核查命令清单

## 核查命令清单

```bash
# 1. 架构检查脚本（含 DTO 业务方法检查 + 文件命名冗余后缀检查）
bash scripts/check-architecture.sh

# 2. controller 包 c.JSON 直写（实测 0 处）
grep -rn "c\.JSON(\|c\.AbortWithStatusJSON(" user-server/internal/controller --include="*.go"

# 3. DTO 业务方法（实测 0 处）
grep -rnE "^func \([^)]*\*?[A-Z][a-zA-Z]+\)" user-server/internal/dto --include="*.go" | grep -vE "Value\(|Scan\("

# 4. migrations 文件清单
ls migrations/*.sql

# 5. 前端 api 文件命名
ls user-web/src/api/*.js

# 6. controller 后缀使用情况（应为 0）
ls user-server/internal/controller/*_controller.go 2>/dev/null | wc -l
ls user-server/internal/service/*_service.go 2>/dev/null | wc -l
ls user-server/internal/repository/*_repository.go 2>/dev/null | wc -l
```

## P1 缺口修复进展（2026-07-26）

### P1-1 ✅ Element Plus 图标按需导入
- 删除 `main.js` 中 `import * as ElementPlusIconsVue` 全量命名空间
- 在 `vite.config.js` 增加 `ElementPlusIconResolver` + `unplugin-vue-components`
- 新建 `src/utils/iconMap.js` 显式声明路由用到的 52 个图标
- `elementPlus` chunk 从 1000.93 kB 降至 887.11 kB（-113 kB / -11.4%）

### P1-2 ✅ DTO 业务方法迁移至包级函数
- 14 个 DTO 方法（`Validate()` / `ToModel()` / `FromXxxModel()`）改为包级函数
- 在 `check-architecture.sh` 增加 §5.3 检测项，禁止 DTO 写方法体（仅允许 `Value()` / `Scan()`）

### P1-3 ⚠️ request 导入风格统一（部分完成）
**评估结论：保守路线，仅约束新增，不强制改造存量。**

- ✅ 在 [src/utils/request.js](../../user-web/src/utils/request.js) 顶部补充规范注释，明确：
  - 新代码必须 `import { http } from '@/utils/request'`
  - `default` 导出仅为兼容旧代码保留
- ✅ 新建 [eslint.config.recommended.mjs](../../user-web/eslint.config.recommended.mjs)（参考用，未启用）：
  - `no-restricted-imports` 禁止 default 导入 `@/utils/request`
  - 43 个存量文件通过 overrides 临时放行
  - 提供 4 步启用清单（npm install / 重命名 / scripts / CI）
- ⏸️ **未启用 ESLint**：原因如下
  - 项目当前无 ESLint 依赖，新增需 `npm install -D eslint eslint-plugin-vue`（约 +30 MB node_modules）
  - 全量 lint 修复需逐文件检查 43 个 api 文件的调用语法（`request.get(url, {params})` vs `http.get(url, params)`），风险高、收益低
  - 现有 `vite build` 已能正常出包，引入 lint 不应阻断构建
- 📋 **未来迁移路径**：等下次大改 api 文件时，顺手把 `import request from` 改为 `import { http } from` + 调用语法适配，逐步消化 overrides 白名单

### P1-4 ✅ migrations README 修正
**结论：原报告 "两个 017" 误报，实际只有一个 017；空号 019-023/028 不补占位。**

- 重写 [migrations/README.md](../../migrations/README.md)：
  - 修正执行顺序清单为 `002-033`（原 README 仅列到 017，且误报 017 重复）
  - 显式标注 `019-023`、`028` 为历史空号（合并/回退产生，不补占位文件）
  - 修正 `001_team_user_management.sql` 已被 `025_unify_system_users.sql` 取代的说明
  - 文件清单与 `ls migrations/*.sql` 完全对齐
- ✅ 不创建占位 SQL 文件：理由如下
  - 占位文件无 DDL，纯空跑无业务价值
  - 应用层 `internal/pkg/utils/db/migrate.go` 按文件名顺序执行，跳过空号不会报错
  - 补占位反而污染迁移历史，增加未来清理成本

### P1-5 ✅ controller/service/repository 文件后缀重命名
**结论：原任务仅列 42 个 controller，实际扩展到全层共 140 个文件。**

- ✅ 在 [scripts/check-architecture.sh](../../scripts/check-architecture.sh) §6 增加 `_<layer>.go` 冗余后缀检测：
  - 检测 `controller/_controller.go` / `service/_service.go` / `repository/_repository.go` / `dto/_dto.go`
  - 检测对应的 `_test.go` 文件
- ✅ `git mv` 重命名 140 个文件（保留 git history）：
  - controller: 42 源 + 16 测试 = 58 个
  - repository: 27 源 + 3 测试 = 30 个
  - service: 38 源 + 7 测试 = 45 个（含 `i18n/` 子目录 5 个、`self_learning/` 子目录 2 个）
- ✅ `go build ./...` 通过
- ✅ `go vet ./...` 通过
- ✅ `bash scripts/check-architecture.sh` 全绿（9 项检查全部通过）
- 📋 **未发现冲突**：所有 `<domain>_controller.go` → `<domain>.go` 的目标文件名均不存在（无同名源文件）
- 📋 **未影响功能**：Go 编译以 package 为单位，文件名变更不影响 import 路径

## 改进路线图（P2 阶段）

| 编号 | 项目 | 优先级 | 风险 | 说明 |
|------|------|--------|------|------|
| P2-1 | 存量 43 个 api 文件迁移到 `{ http }` | 中 | 中 | 逐文件改 import + 调用语法，需配测试 |
| P2-2 | 启用 ESLint（重命名 `eslint.config.recommended.mjs` → `eslint.config.mjs`） | 中 | 低 | 删除 overrides 块后即可启用 |
| P2-3 | 前端 `vite-plugin-style-import` 按 CSS 按需引入 | 低 | 中 | 当前 `main.js` 仍全量引入 `element-plus/dist/index.css` |

## 结论

P1-1 / P1-2 / P1-4 / P1-5 已全部完成；P1-3 完成 50%（规范注释 + ESLint 配置文件就位，未启用）。架构检查脚本 9 项全部通过，go build / go vet 均无报错。

> 本审计文档与 [GO_FIVE_LAYER_ARCHITECTURE.md](../architecture/GO_FIVE_LAYER_ARCHITECTURE.md) 配套使用，CI 集成入口：`bash scripts/check-architecture.sh`。
