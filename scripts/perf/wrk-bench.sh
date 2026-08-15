#!/bin/bash
# wrk 性能基准测试（2026-08-15 M3-P1-E9）
#
# 覆盖通用 HTTP API（非 Bridge）
# wrk 比 k6 更轻量，适合纯 HTTP 压测
#
# 用法：
#   brew install wrk  # macOS
#   apt install wrk   # Linux
#
#   ./scripts/perf/wrk-bench.sh http://localhost:8204

set -euo pipefail

TARGET="${1:-http://localhost:8204}"
DURATION="${DURATION:-30s}"
THREADS="${THREADS:-4}"
CONNECTIONS="${CONNECTIONS:-100}"

echo "=== wrk 性能基准测试 ==="
echo "Target: $TARGET"
echo "Duration: $DURATION, Threads: $THREADS, Connections: $CONNECTIONS"
echo

# 1. /healthz（最轻量，验证纯 HTTP 性能上限）
echo "[1/5] GET /healthz"
wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" --latency \
    "$TARGET/healthz"

echo
echo "[2/5] GET /api/health (ready check)"
wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" --latency \
    "$TARGET/api/health"

echo
echo "[3/5] GET /api/bridge/outbox (长轮询，1s 超时)"
wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" --latency \
    -H "Authorization: Bearer ${BRIDGE_TOKEN:-test-token}" \
    -H "X-Channel: xiaohongshu" \
    "$TARGET/api/bridge/outbox?long_poll=1"

echo
echo "[4/5] POST /api/bridge/ingest (单条消息)"
cat > /tmp/ingest-payload.json << 'EOF'
{
  "channel": "xiaohongshu",
  "account_id": "bench-account",
  "agent_id": "bench-agent",
  "conversation_id": "bench-conv",
  "messages": [
    {
      "msg_id": "msg-001",
      "event_id": "evt-001",
      "sender_type": "customer",
      "text": "性能测试消息",
      "timestamp": 1700000000000
    }
  ]
}
EOF

wrk -t"$THREADS" -c"$CONNECTIONS" -d"$DURATION" --latency \
    -H "Authorization: Bearer ${BRIDGE_TOKEN:-test-token}" \
    -H "Content-Type: application/json" \
    -s /tmp/ingest.lua \
    "$TARGET/api/bridge/ingest"

# 5. 阶梯压测（基线对比）
echo
echo "[5/5] 阶梯压测"
for conn in 50 100 200 500; do
    echo "  → $conn connections"
    wrk -t"$THREADS" -c"$conn" -d"10s" --latency \
        "$TARGET/healthz" 2>&1 | grep -E "Latency|Requests/sec"
done

echo
echo "=== 压测完成 ==="
