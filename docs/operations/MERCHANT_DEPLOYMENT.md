# HiveMtk 用户端 - 部署手册

> 用户端独立部署（私域模式，开源版）
> 适用版本：2026-07-24（已移除 License 授权流程与 OTA 升级）
> 适用对象：商户 / 终端用户

---

## 一、部署模式

HiveMtk 用户端采用**私域独立部署**模式：

- 部署在商户自己的服务器（或私有云、混合云）
- 数据库、推理栈、用户数据全部本地化
- 平台端（安装信息收集、心跳上报）通过 `PLATFORM_API_URL` 低频 HTTPS 调用，数据不落地平台端
- 每个商户独立一套完整系统（user-server + PostgreSQL + Redis + 推理栈）

> **禁止 SaaS / 多租户模式**：无 `merchant_id` 字段，所有数据归属当前部署实例。
> **开源版无 License 校验**：所有功能全开放，无 7 天试用、无授权码、无强制首登改密。

---

## 二、硬件最低要求

| 资源 | 最低 | 推荐（生产）| 备注 |
|------|------|------------|------|
| CPU  | 2 核 | 8 核+      | LLM 推理需 4 核+ |
| 内存 | 4GB  | 16GB+      | 14B 模型需 12GB+ |
| 磁盘 | 50GB | 200GB+     | 模型文件 ~10GB |
| 网络 | 内网 | 公网/FRP   | 公网对话需 HTTPS |

**GPU 加速（可选）**：
- NVIDIA 8GB+（dev 档 3B 模型）
- NVIDIA 16GB+（prod 档 14B 模型）

---

## 三、5 步完成部署

### 步骤 1：克隆代码

```bash
git clone https://gitee.com/your-org/hivemtk.git
cd hivemtk
```

### 步骤 2：准备环境变量

```bash
cp .env-example .env

# 关键变量（必须修改）
POSTGRES_PASSWORD=$(openssl rand -hex 24)
JWT_SECRET=$(openssl rand -hex 32)
PLATFORM_ADMIN_PASSWORD=$(openssl rand -hex 16)

# 平台端地址（同机部署用 127.0.0.1）
PLATFORM_API_URL=http://127.0.0.1:8205
# 跨机/生产：改为平台端公网域名，如 https://api.example.com
```

> 开源版**无需**生成 `PLATFORM_LICENSE_SECRET`、`LicenseKey` 等授权相关密钥（已下线）。

### 步骤 3：启动本地推理栈

```bash
# 方式 A：宿主机 llama.cpp 推理栈（推荐，零虚拟化损耗）
bash scripts/inference-host/install-llama-cpp.sh
HIVEMTK_PROFILE=dev bash scripts/inference-host/download-models.sh
bash scripts/inference-host/start-all.sh

# 方式 B：Docker 推理栈（向后兼容）
# 模型档位在 .env 中配置：默认 dev 轻量档（Qwen2.5-3B-Instruct + bge-m3 + bge-reranker-v2-m3）
make inference-up
```

等待三个推理服务（LLM/Embedding/Rerank）health-check 通过后继续。

### 步骤 4：启动用户端服务

```bash
# 方式 A：宿主机开发模式（推荐，air 热更新）
cd user-server && air

# 方式 B：Docker 全栈模式
# 先构建前端（仅首次部署或前端有更新时需要）
cd user-web && npm install && npm run build && cd ..
cd embed-sdk && npm install && npm run build && cd ..

# 启动后端
docker compose up -d
```

### 步骤 5：完成初始化流程

浏览器访问 `http://your-server-ip:8204/setup`：

1. 创建超级管理员账号（用户名 + 密码 + 选填联系方式）
2. 提交后立即生效，系统进入 `INITIALIZED` 状态
3. 跳转登录页，用刚创建的超管账号登录

> **开源版特性**：
> - 无需输入 LicenseKey
> - 无 7 天免费试用限制（功能全开放）
> - 无强制首登改密（commit 65079e5 已移除 `must_change_password` 机制）
> - 详见 [MERCHANT_INITIALIZATION_FLOW.md](MERCHANT_INITIALIZATION_FLOW.md)

---

## 四、关键端口

| 端口 | 服务 | 说明 |
|------|------|------|
| 8204 | user-server API | RESTful + WebSocket |
| 8202 | PostgreSQL (user_db) | 容器内端口 |
| 8203 | Redis | 容器内端口 |
| 8207 | mtk-llm | llama.cpp 推理 |
| 8208 | mtk-embedding | bge-m3 (1024 维) |
| 8209 | mtk-rerank | bge-reranker-v2-m3 |

---

## 五、数据持久化

通过命名卷持久化：

| 卷名 | 用途 |
|------|------|
| `mtk_user_pg_data` | PostgreSQL 数据 |
| `mtk_user_redis_data` | Redis 数据 |
| `mtk_user_logs` | 应用日志 |
| `mtk_user_uploads` | 用户上传文件 |
| `mtk_user_data` | install.lock 等运行时文件 |

> **不要**用 bind mount 替换这些卷，否则数据可能丢失。

---

## 六、运维命令

```bash
# 查看服务状态
docker compose ps

# 查看日志
docker compose logs -f user-server

# 重启服务
docker compose restart user-server

# 停止并保留数据
docker compose down

# 停止并清理数据（危险）
docker compose down -v
```

---

## 七、备份与恢复

### 备份

```bash
# 备份 PostgreSQL
docker exec mtk-user-postgres pg_dump -U admin user_db > backup-$(date +%Y%m%d).sql

# 备份 install.lock（重要！丢失则需重新初始化）
docker cp mtk-user-server:/app/data/install.lock ./install.lock.bak

# 备份运行时文件
docker run --rm -v mtk_user_data:/data -v $(pwd):/backup alpine tar czf /backup/data-backup-$(date +%Y%m%d).tar.gz -C /data .
```

### 恢复

```bash
# 恢复 PostgreSQL
cat backup-20260721.sql | docker exec -i mtk-user-postgres psql -U admin -d user_db

# 恢复 install.lock
docker cp ./install.lock.bak mtk-user-server:/app/data/install.lock
```

---

## 八、升级

> 开源版采用**纯 git 提交推送**发布：客户自行 `git pull` 拉取升级，无 OTA 下发。

```bash
# 1. 备份数据
make backup  # 或执行上面的备份命令

# 2. 拉取新代码
git pull

# 3. 重新构建前端（如有变更）
cd user-web && npm install && npm run build && cd ..
cd embed-sdk && npm install && npm run build && cd ..

# 4. 重新构建并启动
docker compose build
docker compose up -d

# 5. 执行新迁移
docker exec -i mtk-user-postgres psql -U admin -d user_db < migrations/0XX_*.sql
```

---

## 九、常见问题

### 9.1 RAG 检索失败

```bash
# 检查 embedding 服务
curl http://localhost:8208/health
# 检查向量维度（必须 1024）
docker exec mtk-user-postgres psql -U admin -d user_db \
  -c "SELECT column_name, format_type(udt_name, udt_typmod) FROM information_schema.columns WHERE table_name='knowledge_embeddings' AND column_name='embedding';"
```

### 9.2 LLM 响应慢

```bash
# 检查推理服务状态
curl http://localhost:8207/health
# 检查 GPU 是否被使用
docker exec mtk-llm nvidia-smi
```

### 9.3 平台端心跳上报失败

> 平台端心跳为 best-effort，失败仅 Warn 日志，**不阻塞**本地业务。

```bash
# 检查 PLATFORM_API_URL 配置
docker exec mtk-user-server env | grep PLATFORM
# 手动测试连通性
docker exec mtk-user-server wget -qO- $PLATFORM_API_URL/health
```

---

## 十、迁移到生产

参考 `architecture/部署方案_用户端.md` 了解详细论证。

生产环境额外建议：

1. **HTTPS 终结**：使用外部反代（CDN / 云负载均衡），自行配置 TLS 证书
2. **公网 IP / FRP 穿透**：无公网 IP 时使用 FRP 把 8204 端口穿透出去
3. **备份策略**：每日全量备份 PostgreSQL，每周异地备份
4. **监控告警**：接入 Prometheus / Grafana，关注 user-server 健康指标
5. **日志收集**：使用 ELK / Loki 等集中收集日志

---

## 十一、技术支持

- 官网：https://hivemtk.com
- 文档：本目录 `docs/INDEX.md`
- 邮箱：support@hivemtk.com
- 开源仓库：
  - GitHub：https://github.com/xiaofang142/hivemtk
  - Gitee：https://gitee.com/xhpmayun/hivemtk
