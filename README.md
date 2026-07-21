# HivemTK 用户端

> HivemTK 用户端 - 私域独立部署的智能营销自动化平台

## 项目简介

HivemTK 是一套面向中小企业的智能营销自动化工具，提供：
- **多渠道客户管理 (CDP)**: 抖音/快手/小红书/咸鱼/微信/短信/邮件全渠道触达
- **智能体客服**: AI 智能体自动应答 + 人工接管
- **RAG 知识库**: 本地化检索增强生成，数据不出域
- **智能卡片**: 抖音/快手/小红书/咸鱼四平台卡片自动生成
- **数据看板**: 实时业务监控 + 自定义报表

**核心价值**:所有数据本地化部署，**绝不上云**，符合数据合规要求。

## 仓库结构

```
hivemtk/                          # 用户端仓库
├── user-server/                  # Go 后端服务（核心业务）
├── user-web/                     # Vue 3 前端（B 端工作台）
├── embed-sdk/                    # 嵌入 SDK（Web Widget）
├── migrations/                   # 数据库迁移 SQL
├── docs/                         # 项目文档
├── offline-deploy/               # 私域离线部署脚本
├── models/                       # 本地 AI 模型（.gitkeep，运行时下载）
├── nginx/                        # Nginx 反代配置
├── scripts/                      # 运维脚本
├── tests/                        # 自动化测试
├── .github/                      # CI/CD 工作流
├── docker-compose.yml            # 服务编排（业务栈 + 本地推理栈）
├── docker-compose-example.yml    # 服务编排示例（纳入版本追踪）
├── Makefile                      # 常用命令
└── .env-example                  # 环境变量模板（纳入版本追踪）
```

## 快速开始

### 前置要求

- Docker 24+ & Docker Compose v2
- 4 核 CPU / 8GB 内存 / 50GB 磁盘（最低配置）
- 8 核 CPU / 16GB 内存 / 100GB 磁盘（推荐配置，含 LLM）

### 5 分钟上手

```bash
# 1. 克隆仓库
git clone git@gitee.com:xhpmayun/hivemtk.git
cd hivemtk

# 2. 一键安装（自动构建前端 + 拉起服务）
make install

# 3. 编辑 .env 配置密钥
vim .env
# 必填：
#   POSTGRES_PASSWORD   强密码
#   JWT_SECRET         openssl rand -hex 32
#   PLATFORM_LICENSE_SECRET  openssl rand -hex 32

# 4. 启动所有服务
make up

# 5. 访问
# 用户端: http://localhost:8204
# 默认管理员: admin / (在 .env 中设置 ADMIN_PASSWORD)
```

### 详细部署

详见 [docs/operations/本地私有化离线AI营销客服工具冷启动详细执行文档.md](docs/operations/本地私有化离线AI营销客服工具冷启动详细执行文档.md)

## 与平台端的关系

本仓库为**用户端**，需要与**平台端**(platform-server)配合使用：
- 用户端：部署在客户本地，存储所有业务数据
- 平台端：部署在平台运营方云端，提供 License 验证、版本更新等服务

**平台端仓库**：[gitee.com/xhpmayun/hivemtk-platform](https://gitee.com/xhpmayun/hivemtk-platform)

## 私域部署

支持完全离线部署（无外网），详见 [docs/FRP私域部署指南.md](docs/FRP私域部署指南.md)

## License

本项目为**商业软件**，7 天免费试用。详见 [LICENSE](LICENSE)。

如需正式授权，请联系商务。

## 文档导航

- [产品 PRD](docs/PRODUCT_PRD.md)
- [架构图](docs/standards/ARCHITECTURE_DIAGRAM.md)
- [API 契约](docs/standards/API_CONTRACT.md)
- [前端规范](docs/standards/FRONTEND_CODING_STANDARDS.md)
- [后端规范](docs/standards/BACKEND_CODING_STANDARDS.md)
- [部署指南](docs/operations/本地私有化离线AI营销客服工具冷启动详细执行文档.md)
- [CHANGELOG](CHANGELOG.md)
