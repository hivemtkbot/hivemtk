# HiveMtk Helm Chart

> **任务编号**: OPT-MISC-01
> **状态**: 骨架 (skeleton) — `helm lint` 通过, 尚未经 staging 实测
> **创建日期**: 2026-08-16

## 包含内容

```
hivemtk/
├── Chart.yaml              # Chart 元数据
├── values.yaml             # 默认配置
├── README.md               # 本文件
└── templates/
    ├── deployment.yaml     # user-server Deployment
    ├── service.yaml        # ClusterIP Service
    └── ingress.yaml        # 路由规则
```

## 快速开始

```bash
# 1) 渲染预览
helm template hivemtk ./deploy/helm/hivemtk

# 2) 静态检查
helm lint ./deploy/helm/hivemtk

# 3) 安装到 k8s (需先准备 secrets)
kubectl create secret generic hivemtk-secrets \
  --from-literal=db-host=postgres \
  --from-literal=db-password=YOUR_DB_PASSWORD \
  --from-literal=jwt-secret=$(openssl rand -hex 32) \
  --from-literal=field-encryption-key=$(openssl rand -hex 32)

helm install hivemtk ./deploy/helm/hivemtk \
  --namespace hivemtk --create-namespace
```

## 自定义 values

```bash
helm install hivemtk ./deploy/helm/hivemtk \
  -f my-prod-values.yaml \
  --set replicaCount=3 \
  --set image.tag=v0.2.0
```

最小 `my-prod-values.yaml`:

```yaml
replicaCount: 3
ingress:
  hosts:
    - host: api.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: hivemtk-tls
      hosts:
        - api.example.com
```

## 已知限制 / 待办

- [ ] **未实现** HPA (autoscaling.enabled 骨架默认 false)
- [ ] **未实现** PodDisruptionBudget (podDisruptionBudget.enabled 骨架默认 false)
- [ ] **未实现** ServiceAccount / NetworkPolicy
- [ ] **未实现** PostgreSQL/Redis 子 Chart 依赖
- [ ] **未实现** platform-server / user-web 子 Chart
- [ ] **未测试** helm lint 之外的 staging 验证 (后续 OPT-MISC-04 ~ OPT-MISC-06 实施)
- [ ] **未集成** secrets 轮换 (OPT-SEC-08 策略文档另议)

## 设计原则

1. **最小骨架原则**: 仅含 Deployment + Service + Ingress, 后续按 OPT 任务增量
2. **配置外置**: 所有可变项集中在 `values.yaml`, 不在模板硬编码
3. **Secret 零明文**: 密钥引用 `existingSecret` (`hivemtk-secrets`), 真实密钥由外部系统 (Vault / ExternalSecrets) 同步
4. **标签规范**: 遵循 [k8s 通用标签](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/)
