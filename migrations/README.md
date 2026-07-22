# 数据库迁移执行顺序

> 最后更新：2026-07-22  
> 适用范围：hivemtk 用户端 PostgreSQL 数据库（`user_db` / `mtk-postgres` 容器）

## 总览

本目录包含用户端的所有 SQL 迁移文件，分两类：

| 类型 | 文件 | 说明 |
|------|------|------|
| **基础初始化** | `init-user-db.sql` | 启用扩展（pgvector / uuid-ossp）+ 核心表结构（知识库 / RAG 产品） |
| **增量迁移** | `001_*.sql` ~ `017_*.sql` | 按版本顺序应用的功能迁移 |

## 执行顺序

```text
1. init-user-db.sql              ← 基础结构（首次部署必跑）
   ↓
2. 001_team_user_management.sql  ← 团队用户表
3. 002_ai_content.sql            ← AI 内容生成
4. 003_unified_message.sql       ← 统一消息
5. 004_customer_session.sql      ← 客户会话
6. 005_rfm_user_segment.sql      ← RFM 用户分群
7. 006_custom_reports.sql        ← 自定义报表
8. 007_integration.sql           ← 集成
9. 008_ab_test.sql               ← A/B 测试
10. 009_churn_prediction.sql     ← 客户流失预测
11. 010_satisfaction_surveys.sql ← 满意度调研
12. 011_ai_sales_champion.sql    ← AI 销冠
13. 012_rag_enhancement.sql      ← RAG 增强
14. 013_version_offline_fields.sql← 版本/离线字段
15. 014_site_contact_config.sql  ← 站点联系配置
16. 015_init_flow_enhancement.sql← 初始化流程增强
17. 016_merchants_key_length.sql ← 商户密钥长度
18. 017_*.sql                    ← CDE P1 修复 & 客服增强（两文件独立，无强依赖）
```

## 关键约束

- **顺序性**：必须按上述编号顺序执行；后续迁移可能引用前序表的字段、索引、约束。
- **幂等性**：所有迁移文件均使用 `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` 等幂等语法，可安全重跑。
- **`init-user-db.sql` 与 001-017 的关系**：
  - `init-user-db.sql` 创建**核心基础设施表**（pgvector 扩展、知识库向量表、RAG 产品表等），是其他迁移的前置。
  - `001-017` 创建**业务功能表**，必须在 `init-user-db.sql` 之后执行。
- **重复编号 `017_*.sql`**：当前有 `017_cde_p1_gap_fixes.sql` 与 `017_customer_service_enhancements.sql` 两个文件，编号重复但内容互补。建议：
  - 首次部署按字母顺序先跑 `017_cde_p1_gap_fixes.sql`，再跑 `017_customer_service_enhancements.sql`。
  - 两个文件之间无强依赖，顺序可互换。
  - 后续新增迁移应避免重复编号（建议改名 `018_*.sql`）。

## Docker / 自动执行路径

`docker-compose.yml` 中 `mtk-postgres` 容器的 `initdb.d` 机制：

- 仅 `init-user-db.sql` 通过 `docker-entrypoint-initdb.d/` 自动执行（容器首次创建时）。
- `001-017` 迁移由应用层 `internal/pkg/utils/db/migrate.go` 在 `user-server` 启动时按文件名顺序执行。
- 升级时无需手动重跑已执行迁移；服务重启时 `migrate.go` 会跳过已记录的迁移任务。

## 平台端迁移

平台端（`hivemtk-platform/platform-server`）的数据库迁移**不在本目录**。  
平台端使用 `platform_db` 独立数据库，迁移逻辑嵌入在 `internal/pkg/utils/db/migrate.go` 中，
由 `internal/migration/*.go` 模块管理；不通过 SQL 文件而是通过 GORM 自动建表 + 显式迁移任务。

## 故障排查

| 现象 | 可能原因 | 修复方法 |
|------|----------|----------|
| `relation "xxx" does not exist` | 迁移未按顺序执行 | 按 `001 → 017` 顺序重跑 |
| `column "xxx" already exists` | 迁移被部分执行后中断 | 使用 `IF NOT EXISTS` 重新执行（已支持幂等） |
| `extension "vector" is not available` | pgvector 镜像未启用 | 确认使用 `pgvector/pgvector:pg15` 镜像 |
| `permission denied for extension vector` | 数据库非 superuser | 切换 `admin` / `postgres` 角色建库后，普通用户使用 |
