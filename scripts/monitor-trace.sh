#!/usr/bin/env bash
# 全链路业务监控 CLI —— 调用后端 /api/monitor 端点输出核心链路健康、按渠道节点健康、生命周期追踪。
# 监控端点沿用 bridge 的 InitGuard 私域鉴权模型（无需 JWT），只需能访问 API 即可。
#
# 用法：
#   bash scripts/monitor-trace.sh                  # 健康概览 + 异常 + 按渠道节点健康 + 端到端时延
#   bash scripts/monitor-trace.sh <conversation_id> # 附加该会话的业务链路节点追踪（入参/出参/响应时间/预期/异常）
#   MONITOR_BASE=http://127.0.0.1:8204 bash scripts/monitor-trace.sh
#
# 依赖：curl、jq（无 jq 时退化为原始 JSON 打印）

set -euo pipefail

MONITOR_BASE="${MONITOR_BASE:-http://127.0.0.1:8204}"
CONV="${1:-}"

jq() { command jq "$@" 2>/dev/null || cat; }
urlenc() { python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$1" 2>/dev/null || echo "$1"; }

api() {
  curl -fsS --max-time 8 "$MONITOR_BASE$1" 2>/dev/null || echo "{}"
}

echo "=== hivemtk 全链路业务监控 @ ${MONITOR_BASE} ==="
echo "（可视化 UI：user-web 控制台 → 系统设置 → 链路追踪，或浏览器打开前端站点 /system/trace）"
echo "--- ① 业务链路健康概览 ---"
api "/api/monitor/health" | jq '
  "近1h入站/出站(条·分):  \(.data.inbound_rate_per_min) / \(.data.outbound_rate_per_min)\n" +
  "下行出库队列(pending):  \(.data.pending_count)  (最旧 \(.data.oldest_pending_min) 分钟)\n" +
  "delivered / failed:     \(.data.delivered_count) / \(.data.failed_count)\n" +
  "同步缺口(sync_gap):     \(.data.sync_gap_count)\n" +
  "卡住 可达/不可达:       \((.data.stuck_reachable // []) | length) / \((.data.stuck_unreachable // []) | length)\n" +
  "链路节点异常数:         \(.data.abnormal_count)"
'

echo "--- ② 链路异常 ---"
api "/api/monitor/anomalies" | jq -r '
  (.data.sync_gap // [])[]? | "  [缺口] 会话=\(.conversation_id) 缺口消息=\(.message_count)",
  (.data.stuck_reachable // [])[]? | "  [卡住-可达] 会话=\(.conversation_id) 滞留=\(.age_min)分",
  (.data.stuck_unreachable // [])[]? | "  [卡住-不可达] 会话=\(.conversation_id) 滞留=\(.age_min)分",
  (.data.unreachable // [])[]? | "  [不可达] 会话=\(.conversation_id) 滞留=\(.age_min)分",
  (.data.node_abnormal // [])[]? | "  [节点异常] 渠道=\(.channel) 节点=\(.node) 异常率=\((.abnormal_rate*100|tostring))%"
' 2>/dev/null || echo "  (无异常)"

echo "--- ③ 按渠道 × 节点健康（响应时间 / 异常率）---"
api "/api/monitor/node-health" | jq -r '.data.nodes[]? | "  \(.channel)\t\(.node)\t样本=\(.total)\t异常率=\((.abnormal_rate*100|tostring))%\tavg=\(.avg_duration_ms)ms\tP95=\(.p95_duration_ms)ms"' 2>/dev/null || echo "  (无数据)"

echo "--- ④ 端到端时延（按渠道：上报接入 → 送达确认）---"
api "/api/monitor/latency" | jq -r '.data[]? | "  \(.channel)\tP50=\(.p50_ms)ms\tP95=\(.p95_ms)ms\t样本=\(.sample_size)"' 2>/dev/null || echo "  (无数据)"

if [ -n "$CONV" ]; then
  echo "--- ⑤ 会话业务链路节点追踪: $CONV ---"
  api "/api/monitor/lifecycle?conversation_id=$(urlenc "$CONV")&limit=5" \
    | jq -r '.data[]? | "  trace=\(.trace_id[0:10]) 会话=\(.conversation_id) 渠道=\(.channel) 方向=\(.direction) 节点=\(.node) \(.status) 耗时=\(.duration_ms)ms\(if (.abnormal|length)>0 then " ⚠异常" else "" end)\n      预期: \(.expected)\(if (.error|length)>0 then "\n      异常: \(.error)" else "" end)"' 2>/dev/null || echo "  (该会话暂无链路节点)"
fi
