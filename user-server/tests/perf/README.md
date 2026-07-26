# 性能压测框架

## 概述
本目录提供 `user-server` 的性能压测工具，基于 Go 标准库 `net/http` 实现，无需引入第三方压测工具（如 `wrk`/`vegeta`/`hey`），保证依赖最小化。

## 独立部署
本项目采用私域独立部署模式（单租户），压测目标全部指向单一 `user-server` 实例的 API 接口。

## 快速开始

```bash
# 编译运行(从仓库根目录)
cd user-server
go run ./cmd/perf                          # 跑全部场景
go run ./cmd/perf -scene=login             # 跑指定场景
go run ./cmd/perf -base=http://localhost:8204

# 指定场景
go run ./cmd/perf -scene=login
go run ./cmd/perf -scene=customer-list
go run ./cmd/perf -scene=message-list
go run ./cmd/perf -scene=knowledge-query
go run ./cmd/perf -scene=cdp-event

# 编译为可执行文件
go build -o perf-test ./cmd/perf/
./perf-test -scene=login
```

## 内置场景

| 场景名 | 路径 | 方法 | 并发 | 总请求 | 用途 |
| --- | --- | --- | --- | --- | --- |
| login | `/api/v1/auth/login` | POST | 20 | 500 | 鉴权性能 |
| customer-list | `/api/v1/customer/list` | GET | 50 | 1000 | 客户列表查询 |
| message-list | `/api/v1/message/list` | GET | 50 | 1000 | 消息列表查询 |
| knowledge-query | `/api/v1/knowledge/search` | GET | 30 | 500 | RAG 检索 |
| cdp-event | `/api/v1/events/pageview` | POST | 100 | 2000 | 事件追踪 |

## 指标说明
- **QPS**：每秒请求数（throughput）
- **P50 / P95 / P99**：响应时间分位数
- **Min / Avg / Max**：最小 / 平均 / 最大响应时间
- **Status Codes**：HTTP 状态码分布

## 扩展新场景
在 `main.go` 中新增 `func xxxScene() perf.Config` 并将其加入 `scenes` 切片即可。

## 性能基线（参考）
- 登录接口：P95 < 300ms，QPS > 100
- 列表查询：P95 < 500ms，QPS > 200
- CDP 事件：P95 < 200ms，QPS > 500

> 注：实际基线需根据部署环境硬件配置及数据库性能调整。
