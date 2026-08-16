# HiveMtk 密钥轮换策略

> **配套规则**: [MASTER_RULES.md](../standards/MASTER_RULES.md)

---

## 一、目的

定期轮换所有长期凭证，降低泄露风险。

---

## 二、轮换清单

| 密钥 | 环境变量 | 频率 | 存储位置 |
|------|----------|------|----------|
| JWT 签名 | `JWT_SECRET` | 90 天 | `.env` 文件 |
| 商户 HMAC | `MERCHANT_HMAC_KEY` | 180 天 | 数据库 |
| 字段加密 | `FIELD_ENCRYPTION_KEY` | 180 天 | `.env` 文件 |
| 数据库密码 | `POSTGRES_PASSWORD` | 180 天 | `.env` 文件 |
| Redis 密码 | `REDIS_PASSWORD` | 365 天 | `redis.conf` |

---

## 三、轮换流程

### 3.1 准备阶段

```bash
# 1. 生成新密钥
openssl rand -hex 32

# 2. 备份当前配置
cp .env .env.backup

# 3. 通知相关人员
```

### 3.2 执行轮换

#### JWT 密钥 (90天)

```bash
# 1. 编辑 .env 文件
# JWT_SECRET_OLD=旧值
# JWT_SECRET=新值

# 2. 重启服务
docker compose restart user-server

# 3. 验证新密钥生效
curl -H "Authorization: Bearer <新token>" http://localhost:8204/api/v1/health

# 4. 7天后移除旧密钥
```

#### 数据库密码 (180天)

```bash
# 1. 修改 PostgreSQL 密码
psql -c "ALTER USER hivemtk PASSWORD '新密码';"

# 2. 更新 .env 文件中的 POSTGRES_PASSWORD

# 3. 重启服务
docker compose restart user-server
```

#### Redis 密码 (365天)

```bash
# 1. 修改 Redis 密码
redis-cli CONFIG SET requirepass "新密码"

# 2. 更新 .env 文件中的 REDIS_PASSWORD

# 3. 重启服务
docker compose restart user-server
```

### 3.3 验证

```bash
# 检查服务健康状态
curl http://localhost:8204/healthz

# 检查日志有无错误
docker compose logs --tail=50 user-server | grep -i error
```

---

## 四、紧急轮换

**触发**: 密钥泄露

```bash
# 1. 立即吊销
# 编辑 .env 移除泄露的密钥

# 2. 生成新密钥
NEW_SECRET=$(openssl rand -hex 32)

# 3. 更新配置并重启
# .env: JWT_SECRET=$NEW_SECRET
docker compose restart user-server

# 4. 验证服务正常
curl http://localhost:8204/healthz
```

---

## 五、检查命令

```bash
# 查看密钥文件权限
ls -la .env
chmod 600 .env

# 检查密钥使用时长
# 通过 last_modified 时间戳判断
stat .env
```

---

## 六、参考

- NIST SP 800-57: Key Management
- OWASP Cryptographic Storage Cheat Sheet
