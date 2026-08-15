# ISO/IEC 27001 适用性声明 (Statement of Applicability, SoA)

> **版本:** 1.0
> **日期:** 2026-08-15
> **维护:** HiveMTK 安全合规组
> **依据:** ISO/IEC 27001:2022 附录 A（A.5-A.8）
> **配套:** [GDPR 合规](GDPR.md) · [等保 2.0 三级](DJCP_Level3.md) · [等保2.0三级合规](../operations/等保2.0三级合规.md)

---

## 1. 适用范围

本适用性声明覆盖 **HiveMtk 用户端产品**（hivemtk 仓库：user-server / user-web / user-web/bridge / embed-sdk / 宿主机推理栈）作为**软件产品**的安全控制设计，供部署方（组织）在建立 ISO/IEC 27001 信息安全管理体系（ISMS）时作为**产品侧技术控制项参考**。

> ⚠️ 本文件是**产品技术能力**的适用性说明，不等于组织级 ISMS 认证。组织级认证需结合组织自身管理措施（方针、流程、人员、意识培训）。

**体系环境特征**:
- 部署形态：私域单租户（客户内网 / 客户自有云）
- 数据形态：业务数据零出域；平台端仅元数据
- 可观测形态：无外部监控 SaaS，改应用层日志 + 审计表
- 架构约束：Go 五层架构（Controller → Service → Repository → Model → DTO），禁止跨层

---

## 2. 控制项与实施证据（按 ISO 27001:2022 附录 A 组织）

> 状态图例：**适用且已实施**（✅）/ **适用但需部署方配套**（🧩）/ **不适用**（➖）/ **部分适用**（⚠️）

### A.5 组织控制 (Organizational Controls)

| 控制项 | 状态 | 产品侧实施证据 |
|--------|------|---------------|
| A.5.1 信息安全方针 | 🧩 | 部署方制定；本系列合规文档作为输入 |
| A.5.2 角色与职责 | 🧩 | [Bridge_Runbook.md](../operations/Bridge_Runbook.md) §7 责任人矩阵 |
| A.5.9 供应商管理 | 🧩 | 依赖供应商清单见 [THIRD_PARTY_LICENSES.md](../../THIRD_PARTY_LICENSES.md) |
| A.5.10 可接受使用 | 🧩 | 部署方制定 |
| A.5.15 访问控制 | ✅ | RBAC（admin/manager/agent/user）+ 行级 data_scope |
| A.5.20 供应商协议中的安全 | 🧩 | 云端 LLM 供应商需 DPA/SCC（见 GDPR §4） |
| A.5.25 开发生命周期安全 | ✅ | 五层架构护栏 CI、golangci-lint、Dependabot、SBOM、SLSA、DCO |
| A.5.28 安全编码 | ✅ | `scripts/check-architecture.sh` + `.golangci.yml` depguard + 代码评审 |
| A.5.30 外包开发 | 🧩 | 外包合同需含安全要求 |
| A.5.33 测试信息安全 | ✅ | vitest / go test -race / Playwright / 渗透测试（年度第三方） |

### A.6 人员控制 (People Controls)

| 控制项 | 状态 | 产品侧实施证据 |
|--------|------|---------------|
| A.6.3 安全意识培训 | 🧩 | 部署方组织 |
| A.6.5 违规处理 | 🧩 | 部署方制度 |

### A.7 物理控制 (Physical Controls)

| 控制项 | 状态 | 说明 |
|--------|------|------|
| A.7.1-A.7.14 | 🧩 / ➖ | 私域部署物理安全由部署方机房/云环境承担；软件侧不适用 |

### A.8 技术控制 (Technological Controls)

| 控制项 | 状态 | 产品侧实施证据 |
|--------|------|---------------|
| A.8.2 特权访问管理 | ✅ | 三权分立（超管/审计/安全）；`audit_logs` 表禁 UPDATE/DELETE |
| A.8.3 信息访问限制 | ✅ | 行级 data_scope + 渠道账号凭据掩码 |
| A.8.5 身份认证 | ✅ | JWT + TOTP MFA (RFC 6238) + 登录失败锁定（5 次/30min） |
| A.8.8 漏洞管理 | ✅ | Dependabot 每周扫描；Trivy 镜像扫描（release.yml）；gosec |
| A.8.10 恶意代码防护 | ✅ | ClamAV 上传文件扫描 |
| A.8.11 数据备份 | ✅ | `make db-backup` 每日；[DR_RECOVERY.md](../operations/DR_RECOVERY.md) 每周恢复演练 |
| A.8.12 日志与监控 | ✅ | 应用层日志 + `audit_logs`/`layer_decision_logs` 审计表落库（无外部监控替代方案） |
| A.8.15 日志管理 | ✅ | 审计日志保留 ≥ 1 年 + 异地备份 |
| A.8.16 监控活动 | ⚠️ | 私域以巡检 SQL/日志 grep 实现（见 Bridge_Runbook §8） |
| A.8.19 软件安装管控 | ✅ | 依赖锁文件（go.sum / package-lock.json）+ npm ci / go mod |
| A.8.20 网络安全 | ✅ | 端口不暴露公网、`/api/bridge/*` Token 鉴权、Nginx ACL |
| A.8.22 网络隔离 | ✅ | 数据层（8202/8203）与推理栈（8207-8209）内网端口 |
| A.8.23 Web 过滤 | 🧩 | 部署方在边界层配置 |
| A.8.24 密码学 | ✅ | bcrypt(cost=12)、AES-256-GCM（凭据）、TLS 1.2+ |
| A.8.25 开发生命周期安全 | ✅ | 五层护栏 + lint + race + 覆盖率 |
| A.8.26 应用安全 | ✅ | 输入校验（binding:"required" + service 双层）、WAF（部署方）、限流 |
| A.8.27 安全系统架构 | ✅ | 五层架构硬约束 + 零出域 + FRP 私域穿透 |
| A.8.28 安全编码 | ✅ | 同 A.5.28 |
| A.8.29 发布安全测试 | ✅ | CI 全量测试 + 灰度发布（AI_AGENT_PERF_DEPLOY §3） |
| A.8.30 外包开发 | 🧩 | 同 A.5.30 |
| A.8.31 测试数据脱敏 | ⚠️ | 生产数据需脱敏后用于测试；sanitize.go 提供脱敏能力 |
| A.8.32 变更管理 | 🧩 | 部署方变更流程；产品侧提供灰度/回滚（FeatureFlag 一键关闭） |
| A.8.34 保护隐私与 PII | ✅ | 脱敏中间件（手机/邮箱/身份证/银行卡）、GDPR 支撑 |

---

## 3. 残余风险

| 风险 | 状态 | 说明 / 缓解 |
|------|------|------------|
| 组织级 ISMS 管理措施缺失 | 🧩 | 方针/流程/培训/内部审计需部署方建立 |
| 云端 LLM 数据出境 | ⚠️ | 默认本地推理；外部 LLM 需 SCC + 脱敏 |
| 管理控制项依赖人工 | 🧩 | 权限复审、变更审批等需组织流程 |
| 私域无统一 SIEM | ⚠️ | 日志导出能力 + 部署方可对接自建 SIEM |
| 删除权/数据留存未自动化 | ⚠️ | 提供删除/导出接口，流程由部署方配置 |

---

## 4. 与其它合规的映射

| 本 SoA 控制域 | GDPR | 等保 2.0 三级 |
|--------------|------|-------------|
| A.8.24 密码学 | Art.32 安全 | 4.6 数据保密性 |
| A.8.12/8.15 日志 | Art.30 记录 | 4.3 安全审计 |
| A.8.34 隐私保护 | 全篇 | 4.6 数据保密性 + 附录 B 脱敏 |
| A.8.11 备份 | Art.32 | 4.7 备份与恢复 |

---

> 配套：[GDPR](GDPR.md) · [DJCP 等保三级](DJCP_Level3.md) · [等保2.0三级合规](../operations/等保2.0三级合规.md) · [SLA/SLO](../operations/SLA_SLO.md)
