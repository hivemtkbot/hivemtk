# 贡献指南 (Contributing)

> 本仓库为**商业软件**（详见 [LICENSE](LICENSE)），仅在授权范围内接受贡献。

## 1. 适用范围

本仓库为 HivemTK 用户端（hivemtk）源代码仓库，**仅授权给**：
- HivemTK 平台运营方
- 已获得商业授权的客户（仅限自用部署）
- 平台运营方书面授权的第三方开发者

未经书面许可，请勿提交 PR / Issue 之外的衍生作品。

## 2. 开发流程

### 2.1 本地开发环境

- Go 1.25.0
- Node.js 20 LTS
- Docker 24+ & Docker Compose v2
- PostgreSQL 15+（或使用项目自带 docker 容器）

### 2.2 启动本地栈

```bash
# 1. 复制环境变量
cp .env-example .env
# 编辑 .env，至少设置：
#   POSTGRES_PASSWORD、REDIS_PASSWORD、JWT_SECRET、PLATFORM_LICENSE_SECRET

# 2. 启动本地推理栈
make inference-up           # 拉起 mtk-llm / mtk-embedding / mtk-rerank（模型档位在 .env 中配置，默认 prod）

# 3. 构建前端
make web-build              # user-web
make sdk-build              # embed-sdk

# 4. 启动用户端
make up

# 5. 健康检查
curl http://localhost:8204/health
```

### 2.3 代码规范

- 后端 Go 代码：遵循 [docs/standards/BACKEND_CODING_STANDARDS.md](docs/standards/BACKEND_CODING_STANDARDS.md)（如已迁移）
- 前端 Vue 代码：遵循 [docs/standards/FRONTEND_CODING_STANDARDS.md](docs/standards/FRONTEND_CODING_STANDARDS.md)（如已迁移）
- 五层架构：Controller → Service → Repository → Model → DTO，**禁止跨层调用**

### 2.4 测试

- 后端 API：`go test ./...` + Postman/curl 集成测试
- 前端 UI：使用 Playwright，详见 `tests/ui/user/`
- 推理栈连通性：`bash scripts/inference/smoke_test.sh`

### 2.5 提交流程

1. 从 `main` 创建特性分支：`git checkout -b feature/<name>`
2. 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)
3. 推送前自测：`make test` + `make smoke`
4. 推送并创建 Merge Request

## 3. 反馈与支持

- 技术问题：通过授权渠道提交工单
- 安全漏洞：请私下联系 security@hivemtk.com

## 4. 行为准则

- 不接受任何形式的不当言论、骚扰、歧视
- 提交内容须符合中华人民共和国法律法规
- 不接受包含商业秘密的代码（参考 [LICENSE](LICENSE) 第 4 条）

## 5. 相关仓库

- 平台端：[hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform)
- 历史合并仓库：[marketing-tools-kit](https://gitee.com/xhpmayun/marketing-tools-kit)（已归档）
