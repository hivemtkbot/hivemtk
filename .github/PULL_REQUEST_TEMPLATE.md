## 概述
<!-- 用一两句话描述此 PR 解决的问题 -->

## 关联 Issue
<!-- 关联的 Issue 编号，例如 Closes #123 -->

## 改动类型
- [ ] Bug 修复（非破坏性）
- [ ] 新功能（非破坏性）
- [ ] 重构（既不修复 bug 也不添加功能）
- [ ] 文档更新
- [ ] 性能优化
- [ ] 依赖升级
- [ ] 其他

## 五层架构合规（后端）
<!-- 如果涉及 Go 后端改动，请逐条确认 -->
- [ ] Controller 未直访 db / repository
- [ ] Service 未直访 db
- [ ] Model 仅含 GORM Hook/TableName
- [ ] DTO 未反向引用 service
- [ ] 文件命名未使用禁用后缀（utils / common / _v1 / _stub / _2026-*）

## 测试
- [ ] 单测已补充
- [ ] 集成测试已补充
- [ ] 手动验证已通过（描述操作步骤）

## 检查清单
- [ ] `make lint` 通过
- [ ] `go vet ./...` 通过
- [ ] `go build ./...` 通过
- [ ] 架构检查 `bash scripts/check-architecture.sh` 通过
- [ ] 未引入新警告 / lint 错误
- [ ] 已更新相关文档

## 截图（如有 UI 改动）
<!-- 拖入截图 -->

## 备注
<!-- 其他需要 reviewer 关注的信息 -->
