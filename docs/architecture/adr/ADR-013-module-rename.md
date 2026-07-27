# ADR-013：模块名 marketing → user-server 重命名

- **范围**: user-server Go 模块名合规化
- **状态**: 立项中（待执行）
- **关联规范**: GO_FIVE_LAYER_ARCHITECTURE.md §2.1
- **关联 ADR**: ADR-012（config 包迁移，建议同步执行）

---

## 一、背景

### 1.1 现状

user-server 的 `go.mod` 模块名为 `marketing`，但目录名为 `user-server`：

```go
// go.mod
module marketing

go 1.25.0
```

所有 import 路径形如 `marketing/internal/...`，与目录名 `user-server` 不一致。

### 1.2 历史原因

项目早期定位为"营销系统"（marketing），后演进为"HiveMTK 用户端"（user-server）。模块名未同步更新，成为历史遗留。

### 1.3 问题

1. **认知负担**：新人看到目录 `user-server` 但 import `marketing/...` 会困惑
2. **工具链混乱**：IDE 自动补全、跳转、错误提示都基于 `marketing` 而非 `user-server`
3. **开源友好性**：开源后外部贡献者难以理解模块名与目录名的不一致
4. **与 platform-server 不对称**：platform-server 的模块名为 `hivemtk-platform`（与目录一致），user-server 应对称命名

### 1.4 触发事件

2026-07-26 对 `inference_load_test.go` 的全角度审查发现此问题，列为 P1 历史遗留。详见头脑风暴论证报告。

---

## 二、决策

### 2.1 重命名目标

```
旧：module marketing
新：module user-server
```

或考虑更清晰的命名：

```
方案 A：module user-server        （与目录名完全一致，最简）
方案 B：module hivemtk-user       （与 platform-server 的 hivemtk-platform 对称）
方案 C：module hivemtk/user-server （带组织前缀，更规范但需 go.work 或 replace）
```

**推荐方案 B**：`hivemtk-user`，与 `hivemtk-platform` 对称，且避免连字符在 Go 模块名中的潜在问题（Go 模块名允许连字符，但部分工具不友好）。

**最终决策**：待技术委员会确认。本 ADR 默认采用方案 B。

### 2.2 import 路径变更

全量替换：
```
旧：marketing/internal/...
新：hivemtk-user/internal/...
```

预计影响文件数：约 300-500 个（几乎所有 .go 文件）

### 2.3 关联变更

- `go.mod` 的 module 声明
- 所有 `import "marketing/..."` 语句
- 所有 `package` 声明不变（包名与目录名解耦）
- 外部引用（如 platform-server 引用 user-server 的 SDK，需检查）

---

## 三、执行策略

### 3.1 自动化替换

```bash
# 在 user-server 目录下
find . -name "*.go" -exec sed -i '' 's|marketing/internal/|hivemtk-user/internal/|g' {} +
sed -i '' 's|^module marketing$|module hivemtk-user|' go.mod
```

### 3.2 验证

1. `go build ./...` 通过
2. `go test ./...` 通过
3. `go vet ./...` 通过
4. `grep -r "marketing/internal"` 返回 0 结果
5. 检查 platform-server 是否有 `marketing` 的 replace 指令或 import

### 3.3 提交策略

**单次原子提交**：所有 import 替换 + go.mod 修改在一个 commit，避免中间状态不可编译。

---

## 四、风险与缓解

| 风险 | 缓解 |
|---|---|
| 替换遗漏导致编译失败 | `grep -r "marketing/internal"` 兜底，必须返回 0 |
| platform-server 跨工程引用 | 检查 hivemtk-platform 是否有 `replace marketing => ../hivemtk/user-server` |
| 第三方工具依赖模块名 | 检查 Dockerfile、CI 配置、脚本是否有硬编码 `marketing` |
| Git 历史断裂 | 单次原子提交，commit message 清晰说明 |
| 模块名含连字符的兼容性 | Go 1.25+ 完全支持连字符模块名，无兼容性问题 |

---

## 五、与 ADR-012 的协同

ADR-012（config 包迁移）与本 ADR 都涉及全量 import 替换，建议**同步执行**：

1. 先执行 ADR-013（模块名重命名），全量替换 `marketing` → `hivemtk-user`
2. 再执行 ADR-012（config 包迁移），全量替换 `hivemtk-user/internal/pkg/utils/config` → `hivemtk-user/internal/config`

两次原子提交，避免重复劳动。

---

## 六、验收标准

1. `go build ./...` 通过
2. `go test ./...` 通过
3. `go vet ./...` 通过
4. `grep -r "marketing" --include="*.go"` 返回 0 结果（除注释外）
5. `go.mod` 第一行为 `module hivemtk-user`
6. platform-server 的 go.mod 无 `replace marketing` 残留

---

## 七、时间线

- **立项**：2026-07-26（本 ADR）
- **执行**：待排期（建议在下一个 minor 版本切换时执行，与 ADR-012 一起做）
- **完成**：待标记

---

## 八、备选方案（不推荐）

### 8.1 保持现状

**理由**：模块名不影响功能，迁移成本高
**反驳**：开源后认知负担放大，技术债越拖越大

### 8.2 用 go.work 解耦

**理由**：用 go.work 管理 monorepo，模块名可独立
**反驳**：当前 user-server 与 platform-server 是独立仓库，go.work 不适用

---

## 九、关联

- **前置 ADR**：无
- **关联 ADR**：ADR-012（config 包迁移，建议同步执行）
- **关联规范**：GO_FIVE_LAYER_ARCHITECTURE.md §2.1 目录布局
- **触发事件**：2026-07-26 inference_load_test.go 全角度审查头脑风暴
